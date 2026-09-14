// Command headercheck verifies an entire Electroneum chain's headers against
// the real IBFT engine, streaming them over JSON-RPC and holding nothing on
// disk.
//
// This is the primary consensus gate for the geth rebase. It answers one
// question: does THIS tree's consensus code accept every header the live
// network actually produced? Because it drives the production types and the
// production engine -- types.ExtractQBFTExtra, engine.VerifyHeader,
// Backend.GetBaseBlockReward -- a port that changes behaviour anywhere in
// header validation, QBFT extra-data decoding, committed-seal recovery or the
// emission curve fails here, on real mainnet data, at whatever height it first
// diverges.
//
// It costs no chain storage, which is the point: a full sync is ~51 GB, while
// this keeps a two-header window and discards everything else. At ~2.6k
// headers/s over LAN the full 15.7M-block chain takes under two hours.
//
//	go run ./electroswap/etntest/headercheck -rpc http://ai_lan:8545
//	go run ./electroswap/etntest/headercheck -rpc http://ai_lan:8545 -from 0 -to 100000
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math/big"
	"os"
	"time"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/common/math"
	"github.com/electroneum/electroneum-sc/consensus"
	"github.com/electroneum/electroneum-sc/consensus/istanbul"
	istanbulbackend "github.com/electroneum/electroneum-sc/consensus/istanbul/backend"
	istanbulengine "github.com/electroneum/electroneum-sc/consensus/istanbul/engine"
	"github.com/electroneum/electroneum-sc/consensus/istanbul/validator"
	"github.com/electroneum/electroneum-sc/core/types"
	"github.com/electroneum/electroneum-sc/params"
	"github.com/electroneum/electroneum-sc/rpc"
)

func main() {
	var (
		rpcURL    = flag.String("rpc", "http://127.0.0.1:8545", "JSON-RPC endpoint of a synced node")
		from      = flag.Uint64("from", 0, "first block to verify")
		to        = flag.Int64("to", -1, "last block to verify (-1 = current head)")
		batch     = flag.Int("batch", 500, "headers per JSON-RPC batch")
		depth     = flag.Int("depth", 6, "batches to keep in flight")
		sampleN   = flag.Uint64("sample", 25000, "cross-check emission/validators against the node every N blocks (0 = never)")
		failFast  = flag.Bool("failfast", true, "stop at the first divergence")
		maxErrors = flag.Int("maxerrors", 20, "with -failfast=false, stop after this many divergences")
	)
	flag.Parse()

	if err := run(*rpcURL, *from, *to, *batch, *depth, *sampleN, *failFast, *maxErrors); err != nil {
		fmt.Fprintf(os.Stderr, "\nFAIL: %v\n", err)
		os.Exit(1)
	}
}

func run(rpcURL string, from uint64, to int64, batch, depth int, sampleN uint64, failFast bool, maxErrors int) error {
	ctx := context.Background()
	client, err := rpc.DialContext(ctx, rpcURL)
	if err != nil {
		return fmt.Errorf("dial %s: %w", rpcURL, err)
	}
	defer client.Close()

	chainConfig, err := resolveChainConfig(ctx, client)
	if err != nil {
		return err
	}
	if chainConfig.IBFT == nil {
		return fmt.Errorf("chain %v has no IBFT config", chainConfig.ChainID)
	}
	head, err := currentHead(ctx, client)
	if err != nil {
		return err
	}
	if to < 0 || uint64(to) > head {
		to = int64(head)
	}
	last := uint64(to)
	if from > last {
		return fmt.Errorf("-from %d is beyond -to %d", from, last)
	}

	// The emission oracle lives in the `istanbul` RPC namespace, which
	// production exposes only over IPC (--http.api is eth,net,web3,debug). Its
	// absence must not disable the run: we can still track supply with the
	// production reward function, we just cannot reconcile it. Point -rpc at
	// the node's .ipc socket to get the full check.
	emissionOK := true
	if _, err := totalEmission(ctx, client, 1); err != nil {
		emissionOK = false
		fmt.Printf("note: istanbul_getTotalEmission unavailable over this transport.\n"+
			"      Emission will be computed but NOT reconciled against the node.\n"+
			"      For the full check, run on the node against its IPC socket:\n"+
			"        headercheck -rpc /mnt/data/ETN-SC/etn-sc.ipc\n"+
			"      (%v)\n\n", err)
	}

	eng := istanbulengine.NewEngine(istanbulConfig(chainConfig), common.Address{}, nil)
	// GetBaseBlockReward only touches the receiver when circulatingSupply is
	// nil; we always pass it explicitly, so a zero Backend is safe and lets us
	// exercise the real reward code rather than a copy of it.
	rewarder := &istanbulbackend.Backend{}
	chain := newChainReader(chainConfig)

	fmt.Printf("chain %v · verifying blocks %d..%d (%d headers) against %s\n",
		chainConfig.ChainID, from, last, last-from+1, rpcURL)

	var (
		started  = time.Now()
		supply   *big.Int
		verified uint64
		problems int
		prev     *types.Header

		emissionWarned bool
	)

	// A mid-chain range has no parent for its first header, and
	// verifyCascadingFields needs one. Fetch it so -from behaves the same as a
	// run from genesis.
	if from > 0 {
		parents, _, err := fetchRange(ctx, client, from-1, from-1)
		if err != nil {
			return fmt.Errorf("prefetch parent %d: %w", from-1, err)
		}
		prev = parents[0]
		chain.push(prev)
	}

	stream := newStream(ctx, client, from, last, batch, depth)
	for {
		hdrs, hashes, err := stream.next()
		if err != nil {
			return err
		}
		if hdrs == nil {
			break
		}
		for i, h := range hdrs {
			n := h.Number.Uint64()

			// The header we rebuilt from JSON must hash to what the node
			// reported. If this fails, our RLP encoding of the header -- the
			// thing every seal and every child's ParentHash commits to -- is
			// wrong, and nothing below it means anything.
			if got := h.Hash(); got != hashes[i] {
				if err := report(&problems, failFast, maxErrors, n, fmt.Errorf("header hash mismatch: rebuilt %s, node says %s", got, hashes[i])); err != nil {
					return err
				}
				prev = h
				chain.push(h)
				continue
			}
			chain.push(h)

			if n == 0 {
				// Genesis is a valid dead-end for the engine, and its emission
				// record is seeded from config rather than computed.
				supply = new(big.Int).Set(chainConfig.GenesisETN)
				prev = h
				verified++
				continue
			}

			var parents []*types.Header
			if prev != nil && prev.Number.Uint64() == n-1 {
				parents = []*types.Header{prev}
			}

			// The QBFT extra carries the validator set that must have signed
			// this header, so decoding it is itself part of the test: a port
			// that gets the conditional-arity RLP (ProposerSeal) wrong fails
			// right here.
			extra, err := types.ExtractQBFTExtra(h)
			if err != nil {
				if err := report(&problems, failFast, maxErrors, n, fmt.Errorf("ExtractQBFTExtra: %w", err)); err != nil {
					return err
				}
				prev = h
				continue
			}
			valSet := validator.NewSet(extra.Validators, istanbul.NewProposerPolicy(istanbul.ProposerPolicyId(chainConfig.IBFT.ProposerPolicy)))

			if err := eng.VerifyHeader(chain, h, parents, valSet); err != nil {
				if err := report(&problems, failFast, maxErrors, n, fmt.Errorf("VerifyHeader: %w", err)); err != nil {
					return err
				}
				prev = h
				continue
			}

			// Emission is consensus state kept OUTSIDE the state trie, so it
			// is exactly the kind of thing a rebase silently loses. Track it
			// with the production reward function and reconcile against the
			// node's own record at intervals.
			// emissionApply accumulates GetBaseBlockReward over every header from
			// 1..N, so after this line `supply` is emission(N).
			if supply == nil {
				if emissionOK {
					// istanbul_getTotalEmission(N) reports emission(N-1) -- see
					// crossCheck -- which is exactly the seed we need before
					// applying block N's own reward.
					if supply, err = totalEmission(ctx, client, n); err != nil {
						return fmt.Errorf("seed emission at %d: %w", n, err)
					}
				} else if !emissionWarned {
					fmt.Printf("note: cannot seed emission at block %d without the oracle; skipping emission tracking\n", n)
					emissionWarned = true
				}
			}
			if supply != nil {
				supply = new(big.Int).Add(supply, rewarder.GetBaseBlockReward(chain, h, supply))
			}

			if sampleN > 0 && n%sampleN == 0 && supply != nil {
				if err := crossCheck(ctx, client, n, supply, extra.Validators, emissionOK); err != nil {
					if err := report(&problems, failFast, maxErrors, n, err); err != nil {
						return err
					}
				}
			}

			prev = h
			verified++
		}

		if n := prev.Number.Uint64(); n%1_000_000 < uint64(batch) {
			rate := float64(verified) / time.Since(started).Seconds()
			fmt.Printf("  ... %d verified (%.0f hdr/s), supply %s\n", n, rate, formatETN(supply))
		}
	}

	// A final reconciliation at the tip: if the whole run drifted by even one
	// wei, this is where it surfaces.
	if supply != nil {
		if err := crossCheck(ctx, client, last, supply, nil, emissionOK); err != nil {
			problems++
			fmt.Fprintf(os.Stderr, "  block %d: %v\n", last, err)
		}
	}

	elapsed := time.Since(started)
	fmt.Printf("\n%d headers verified in %s (%.0f hdr/s)\n", verified, elapsed.Truncate(time.Second), float64(verified)/elapsed.Seconds())
	if supply != nil {
		fmt.Printf("circulating supply at %d: %s\n", last, formatETN(supply))
	}
	if problems > 0 {
		return fmt.Errorf("%d divergence(s)", problems)
	}
	fmt.Println("OK — every header accepted by this tree's consensus code")
	return nil
}

func report(problems *int, failFast bool, maxErrors int, block uint64, err error) error {
	*problems++
	fmt.Fprintf(os.Stderr, "  block %d: %v\n", block, err)
	if failFast {
		return fmt.Errorf("block %d: %w", block, err)
	}
	if *problems >= maxErrors {
		return fmt.Errorf("stopping after %d divergences", *problems)
	}
	return nil
}

// crossCheck reconciles our independently tracked state against the node's own.
// crossCheck reconciles our independently tracked state against the node's own.
//
// The +1 is not a fudge. Backend.emissionApply folds GetBaseBlockReward over
// headers 1..N, so emission(N) covers block N's reward -- but the RPC reports
// the record as of the PARENT, i.e. istanbul_getTotalEmission(N) == emission(N-1).
// Verified against the live chain across the first halving: block 2,575,608
// pays 4 ETN and 2,575,609 pays 2, which only reconciles under this indexing.
// Getting it wrong shows up as a constant one-block-reward offset that stays
// invisible until the first halving changes the reward size.
func crossCheck(ctx context.Context, client *rpc.Client, n uint64, supply *big.Int, vals []common.Address, emissionOK bool) error {
	if !emissionOK {
		return nil
	}
	want, err := totalEmission(ctx, client, n+1)
	if err != nil {
		return fmt.Errorf("istanbul_getTotalEmission: %w", err)
	}
	if supply.Cmp(want) != 0 {
		return fmt.Errorf("emission mismatch: computed %s, node says %s (delta %s wei)",
			formatETN(supply), formatETN(want), new(big.Int).Sub(supply, want))
	}
	if vals == nil {
		return nil
	}
	var got []common.Address
	if err := client.CallContext(ctx, &got, "istanbul_getValidators", hexUint(n)); err != nil {
		return nil // not fatal: older nodes may not expose it
	}
	if len(got) != len(vals) {
		return fmt.Errorf("validator count mismatch: header says %d, node says %d", len(vals), len(got))
	}
	in := make(map[common.Address]bool, len(got))
	for _, a := range got {
		in[a] = true
	}
	for _, a := range vals {
		if !in[a] {
			return fmt.Errorf("validator %s in header extra but not in node's set", a)
		}
	}
	return nil
}

func totalEmission(ctx context.Context, client *rpc.Client, n uint64) (*big.Int, error) {
	var raw json.RawMessage
	if err := client.CallContext(ctx, &raw, "istanbul_getTotalEmission", hexUint(n)); err != nil {
		return nil, err
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		v, ok := new(big.Int).SetString(trim0x(s), 16)
		if !ok {
			return nil, fmt.Errorf("bad emission value %q", s)
		}
		return v, nil
	}
	var num json.Number
	if err := json.Unmarshal(raw, &num); err != nil {
		return nil, fmt.Errorf("bad emission payload %s", raw)
	}
	v, ok := new(big.Int).SetString(num.String(), 10)
	if !ok {
		return nil, fmt.Errorf("bad emission number %s", num)
	}
	return v, nil
}

func istanbulConfig(c *params.ChainConfig) *istanbul.Config {
	ib := c.IBFT
	return &istanbul.Config{
		RequestTimeoutSeconds:              ib.RequestTimeoutSeconds,
		MaxRequestTimeoutSeconds:           ib.MaxRequestTimeoutSeconds,
		BlockPeriod:                        ib.BlockPeriodSeconds,
		ProposerPolicy:                     istanbul.NewProposerPolicy(istanbul.ProposerPolicyId(ib.ProposerPolicy)),
		Epoch:                              ib.EpochLength,
		AllowedFutureBlockTime:             ib.AllowedFutureBlockTime,
		Transitions:                        c.Transitions,
		PriorityTransactorsContractAddress: c.PriorityTransactorsContractAddress,
	}
}

func resolveChainConfig(ctx context.Context, client *rpc.Client) (*params.ChainConfig, error) {
	var idHex string
	if err := client.CallContext(ctx, &idHex, "eth_chainId"); err != nil {
		return nil, fmt.Errorf("eth_chainId: %w", err)
	}
	id, ok := new(big.Int).SetString(trim0x(idHex), 16)
	if !ok {
		return nil, fmt.Errorf("bad chain id %q", idHex)
	}
	for _, c := range []*params.ChainConfig{
		params.MainnetChainConfig, params.TestnetChainConfig, params.StagenetChainConfig,
	} {
		if c != nil && c.ChainID != nil && c.ChainID.Cmp(id) == 0 {
			return c, nil
		}
	}
	return nil, fmt.Errorf("chain id %v is not a known Electroneum network", id)
}

func currentHead(ctx context.Context, client *rpc.Client) (uint64, error) {
	var s string
	if err := client.CallContext(ctx, &s, "eth_blockNumber"); err != nil {
		return 0, fmt.Errorf("eth_blockNumber: %w", err)
	}
	v, ok := new(big.Int).SetString(trim0x(s), 16)
	if !ok {
		return 0, fmt.Errorf("bad head %q", s)
	}
	return v.Uint64(), nil
}

// ---------------------------------------------------------------------------
// header stream: fetched concurrently, delivered strictly in order
// ---------------------------------------------------------------------------

type chunkResult struct {
	headers []*types.Header
	hashes  []common.Hash
	err     error
}

type stream struct {
	pending chan chan chunkResult
}

func newStream(ctx context.Context, client *rpc.Client, from, to uint64, batch, depth int) *stream {
	s := &stream{pending: make(chan chan chunkResult, depth)}
	go func() {
		defer close(s.pending)
		for start := from; start <= to; start += uint64(batch) {
			end := start + uint64(batch) - 1
			if end > to {
				end = to
			}
			ch := make(chan chunkResult, 1)
			s.pending <- ch
			go func(lo, hi uint64, out chan chunkResult) {
				h, hashes, err := fetchRange(ctx, client, lo, hi)
				out <- chunkResult{h, hashes, err}
			}(start, end, ch)
		}
	}()
	return s
}

func (s *stream) next() ([]*types.Header, []common.Hash, error) {
	ch, ok := <-s.pending
	if !ok {
		return nil, nil, nil
	}
	r := <-ch
	return r.headers, r.hashes, r.err
}

// fetchRange retries transient transport failures. A whole-chain run takes
// hours against a live node, and that node can be restarted, reloaded or simply
// drop an IPC connection under load. Abandoning 2.7 hours of verified headers
// because one batch returned EOF is not a useful failure mode -- a genuine
// consensus divergence is reported by the caller, not by the transport.
func fetchRange(ctx context.Context, client *rpc.Client, lo, hi uint64) ([]*types.Header, []common.Hash, error) {
	const attempts = 12
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			delay := time.Duration(attempt*attempt) * time.Second // 1s, 4s, 9s ... ~6min total
			fmt.Printf("  transport error at %d..%d (%v); retrying in %s\n", lo, hi, lastErr, delay)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			}
		}
		h, hashes, err := fetchRangeOnce(ctx, client, lo, hi)
		if err == nil {
			return h, hashes, nil
		}
		lastErr = err
	}
	return nil, nil, fmt.Errorf("batch %d..%d failed after %d attempts: %w", lo, hi, attempts, lastErr)
}

func fetchRangeOnce(ctx context.Context, client *rpc.Client, lo, hi uint64) ([]*types.Header, []common.Hash, error) {
	n := int(hi-lo) + 1
	elems := make([]rpc.BatchElem, n)
	raws := make([]json.RawMessage, n)
	for i := 0; i < n; i++ {
		elems[i] = rpc.BatchElem{
			Method: "eth_getBlockByNumber",
			Args:   []interface{}{hexUint(lo + uint64(i)), false},
			Result: &raws[i],
		}
	}
	if err := client.BatchCallContext(ctx, elems); err != nil {
		return nil, nil, fmt.Errorf("batch %d..%d: %w", lo, hi, err)
	}
	headers := make([]*types.Header, n)
	hashes := make([]common.Hash, n)
	for i := range elems {
		if elems[i].Error != nil {
			return nil, nil, fmt.Errorf("block %d: %w", lo+uint64(i), elems[i].Error)
		}
		if len(raws[i]) == 0 || string(raws[i]) == "null" {
			return nil, nil, fmt.Errorf("block %d: node returned null", lo+uint64(i))
		}
		var h types.Header
		if err := json.Unmarshal(raws[i], &h); err != nil {
			return nil, nil, fmt.Errorf("block %d: decode header: %w", lo+uint64(i), err)
		}
		var envelope struct {
			Hash common.Hash `json:"hash"`
		}
		if err := json.Unmarshal(raws[i], &envelope); err != nil {
			return nil, nil, fmt.Errorf("block %d: decode hash: %w", lo+uint64(i), err)
		}
		headers[i], hashes[i] = &h, envelope.Hash
	}
	return headers, hashes, nil
}

// ---------------------------------------------------------------------------
// consensus.ChainHeaderReader over a two-header window
// ---------------------------------------------------------------------------

// chainReader satisfies consensus.ChainHeaderReader from a tiny sliding window.
// The IBFT engine only ever asks for Config() and the immediate parent, so
// there is no reason to keep the chain -- which is what makes a whole-chain
// verification fit in a few megabytes of RSS.
type chainReader struct {
	cfg    *params.ChainConfig
	recent []*types.Header
}

func newChainReader(cfg *params.ChainConfig) *chainReader {
	return &chainReader{cfg: cfg, recent: make([]*types.Header, 0, 4)}
}

func (c *chainReader) push(h *types.Header) {
	if len(c.recent) == cap(c.recent) {
		copy(c.recent, c.recent[1:])
		c.recent = c.recent[:len(c.recent)-1]
	}
	c.recent = append(c.recent, h)
}

func (c *chainReader) Config() *params.ChainConfig { return c.cfg }

func (c *chainReader) CurrentHeader() *types.Header {
	if len(c.recent) == 0 {
		return nil
	}
	return c.recent[len(c.recent)-1]
}

func (c *chainReader) GetHeader(hash common.Hash, number uint64) *types.Header {
	for _, h := range c.recent {
		if h.Number.Uint64() == number && h.Hash() == hash {
			return h
		}
	}
	return nil
}

func (c *chainReader) GetHeaderByNumber(number uint64) *types.Header {
	for _, h := range c.recent {
		if h.Number.Uint64() == number {
			return h
		}
	}
	return nil
}

func (c *chainReader) GetHeaderByHash(hash common.Hash) *types.Header {
	for _, h := range c.recent {
		if h.Hash() == hash {
			return h
		}
	}
	return nil
}

func (c *chainReader) GetTd(common.Hash, uint64) *big.Int { return nil }

var _ consensus.ChainHeaderReader = (*chainReader)(nil)

// ---------------------------------------------------------------------------

func hexUint(n uint64) string { return fmt.Sprintf("0x%x", n) }

func trim0x(s string) string {
	if len(s) >= 2 && (s[:2] == "0x" || s[:2] == "0X") {
		return s[2:]
	}
	return s
}

var weiPerETN = math.BigPow(10, 18)

func formatETN(v *big.Int) string {
	if v == nil {
		return "<nil>"
	}
	whole, frac := new(big.Int).QuoRem(v, weiPerETN, new(big.Int))
	return fmt.Sprintf("%s.%018s ETN", whole, frac)
}
