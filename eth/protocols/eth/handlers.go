// Copyright 2021 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package eth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/core"
	"github.com/electroneum/electroneum-sc/core/types"
	"github.com/electroneum/electroneum-sc/crypto"
	"github.com/electroneum/electroneum-sc/log"
	"github.com/electroneum/electroneum-sc/params"
	"github.com/electroneum/electroneum-sc/rlp"
	"github.com/electroneum/electroneum-sc/trie"
)

// handleGetBlockHeaders66 is the eth/66 version of handleGetBlockHeaders
func handleGetBlockHeaders66(backend Backend, msg Decoder, peer *Peer) error {
	// Decode the complex header query
	var query GetBlockHeadersPacket66
	if err := msg.Decode(&query); err != nil {
		return fmt.Errorf("%w: message %v: %v", errDecode, msg, err)
	}
	response := ServiceGetBlockHeadersQuery(backend.Chain(), query.GetBlockHeadersPacket, peer)
	return peer.ReplyBlockHeadersRLP(query.RequestId, response)
}

// ServiceGetBlockHeadersQuery assembles the response to a header query. It is
// exposed to allow external packages to test protocol behavior.
func ServiceGetBlockHeadersQuery(chain *core.BlockChain, query *GetBlockHeadersPacket, peer *Peer) []rlp.RawValue {
	if query.Skip == 0 {
		// The fast path: when the request is for a contiguous segment of headers.
		return serviceContiguousBlockHeaderQuery(chain, query)
	} else {
		return serviceNonContiguousBlockHeaderQuery(chain, query, peer)
	}
}

func serviceNonContiguousBlockHeaderQuery(chain *core.BlockChain, query *GetBlockHeadersPacket, peer *Peer) []rlp.RawValue {
	hashMode := query.Origin.Hash != (common.Hash{})
	first := true
	maxNonCanonical := uint64(100)

	// Gather headers until the fetch or network limits is reached
	var (
		bytes   common.StorageSize
		headers []rlp.RawValue
		unknown bool
		lookups int
	)
	for !unknown && len(headers) < int(query.Amount) && bytes < softResponseLimit &&
		len(headers) < maxHeadersServe && lookups < 2*maxHeadersServe {
		lookups++
		// Retrieve the next header satisfying the query
		var origin *types.Header
		if hashMode {
			if first {
				first = false
				origin = chain.GetHeaderByHash(query.Origin.Hash)
				if origin != nil {
					query.Origin.Number = origin.Number.Uint64()
				}
			} else {
				origin = chain.GetHeader(query.Origin.Hash, query.Origin.Number)
			}
		} else {
			origin = chain.GetHeaderByNumber(query.Origin.Number)
		}
		if origin == nil {
			break
		}
		if rlpData, err := rlp.EncodeToBytes(origin); err != nil {
			log.Crit("Unable to decode our own headers", "err", err)
		} else {
			headers = append(headers, rlp.RawValue(rlpData))
			bytes += common.StorageSize(len(rlpData))
		}
		// Advance to the next header of the query
		switch {
		case hashMode && query.Reverse:
			// Hash based traversal towards the genesis block
			ancestor := query.Skip + 1
			if ancestor == 0 {
				unknown = true
			} else {
				query.Origin.Hash, query.Origin.Number = chain.GetAncestor(query.Origin.Hash, query.Origin.Number, ancestor, &maxNonCanonical)
				unknown = (query.Origin.Hash == common.Hash{})
			}
		case hashMode && !query.Reverse:
			// Hash based traversal towards the leaf block
			var (
				current = origin.Number.Uint64()
				next    = current + query.Skip + 1
			)
			if next <= current {
				infos, _ := json.MarshalIndent(peer.Peer.Info(), "", "  ")
				peer.Log().Warn("GetBlockHeaders skip overflow attack", "current", current, "skip", query.Skip, "next", next, "attacker", infos)
				unknown = true
			} else {
				if header := chain.GetHeaderByNumber(next); header != nil {
					nextHash := header.Hash()
					expOldHash, _ := chain.GetAncestor(nextHash, next, query.Skip+1, &maxNonCanonical)
					if expOldHash == query.Origin.Hash {
						query.Origin.Hash, query.Origin.Number = nextHash, next
					} else {
						unknown = true
					}
				} else {
					unknown = true
				}
			}
		case query.Reverse:
			// Number based traversal towards the genesis block
			if query.Origin.Number >= query.Skip+1 {
				query.Origin.Number -= query.Skip + 1
			} else {
				unknown = true
			}

		case !query.Reverse:
			// Number based traversal towards the leaf block
			query.Origin.Number += query.Skip + 1
		}
	}
	return headers
}

func serviceContiguousBlockHeaderQuery(chain *core.BlockChain, query *GetBlockHeadersPacket) []rlp.RawValue {
	count := query.Amount
	if count == 0 {
		// A zero-amount query returns no headers. Guard against the
		// underflow of count-1 in hash mode, which would otherwise wrap to
		// a huge uint64 and be clamped into serving the whole canonical
		// prefix instead of an empty response.
		return nil
	}
	if count > maxHeadersServe {
		count = maxHeadersServe
	}
	if query.Origin.Hash == (common.Hash{}) {
		// Number mode, just return the canon chain segment. The backend
		// delivers in [N, N-1, N-2..] descending order, so we need to
		// accommodate for that.
		from := query.Origin.Number
		if !query.Reverse {
			from = from + count - 1
		}
		headers := chain.GetHeadersFrom(from, count)
		if !query.Reverse {
			for i, j := 0, len(headers)-1; i < j; i, j = i+1, j-1 {
				headers[i], headers[j] = headers[j], headers[i]
			}
		}
		return headers
	}
	// Hash mode.
	var (
		headers []rlp.RawValue
		hash    = query.Origin.Hash
		header  = chain.GetHeaderByHash(hash)
	)
	if header != nil {
		rlpData, _ := rlp.EncodeToBytes(header)
		headers = append(headers, rlpData)
	} else {
		// We don't even have the origin header
		return headers
	}
	num := header.Number.Uint64()
	if !query.Reverse {
		// Theoretically, we are tasked to deliver header by hash H, and onwards.
		// However, if H is not canon, we will be unable to deliver any descendants of
		// H.
		if canonHash := chain.GetCanonicalHash(num); canonHash != hash {
			// Not canon, we can't deliver descendants
			return headers
		}
		descendants := chain.GetHeadersFrom(num+count-1, count-1)
		for i, j := 0, len(descendants)-1; i < j; i, j = i+1, j-1 {
			descendants[i], descendants[j] = descendants[j], descendants[i]
		}
		headers = append(headers, descendants...)
		return headers
	}
	{ // Last mode: deliver ancestors of H
		for i := uint64(1); header != nil && i < count; i++ {
			header = chain.GetHeaderByHash(header.ParentHash)
			if header == nil {
				break
			}
			rlpData, _ := rlp.EncodeToBytes(header)
			headers = append(headers, rlpData)
		}
		return headers
	}
}

func handleGetBlockBodies66(backend Backend, msg Decoder, peer *Peer) error {
	// Decode the block body retrieval message
	var query GetBlockBodiesPacket66
	if err := msg.Decode(&query); err != nil {
		return fmt.Errorf("%w: message %v: %v", errDecode, msg, err)
	}
	response := ServiceGetBlockBodiesQuery(backend.Chain(), query.GetBlockBodiesPacket)
	return peer.ReplyBlockBodiesRLP(query.RequestId, response)
}

// ServiceGetBlockBodiesQuery assembles the response to a body query. It is
// exposed to allow external packages to test protocol behavior.
func ServiceGetBlockBodiesQuery(chain *core.BlockChain, query GetBlockBodiesPacket) []rlp.RawValue {
	// Gather blocks until the fetch or network limits is reached
	var (
		bytes  int
		bodies []rlp.RawValue
	)
	for lookups, hash := range query {
		if bytes >= softResponseLimit || len(bodies) >= maxBodiesServe ||
			lookups >= 2*maxBodiesServe {
			break
		}
		if data := chain.GetBodyRLP(hash); len(data) != 0 {
			bodies = append(bodies, data)
			bytes += len(data)
		}
	}
	return bodies
}

func handleGetNodeData66(backend Backend, msg Decoder, peer *Peer) error {
	// Decode the trie node data retrieval message
	var query GetNodeDataPacket66
	if err := msg.Decode(&query); err != nil {
		return fmt.Errorf("%w: message %v: %v", errDecode, msg, err)
	}
	response := ServiceGetNodeDataQuery(backend.Chain(), query.GetNodeDataPacket)
	return peer.ReplyNodeData(query.RequestId, response)
}

// ServiceGetNodeDataQuery assembles the response to a node data query. It is
// exposed to allow external packages to test protocol behavior.
func ServiceGetNodeDataQuery(chain *core.BlockChain, query GetNodeDataPacket) [][]byte {
	// Gather state data until the fetch or network limits is reached
	var (
		bytes int
		nodes [][]byte
	)
	for lookups, hash := range query {
		if bytes >= softResponseLimit || len(nodes) >= maxNodeDataServe ||
			lookups >= 2*maxNodeDataServe {
			break
		}
		// Retrieve the requested state entry
		entry, err := chain.TrieNode(hash)
		if len(entry) == 0 || err != nil {
			// Read the contract code with prefix only to save unnecessary lookups.
			entry, err = chain.ContractCodeWithPrefix(hash)
		}
		if err == nil && len(entry) > 0 {
			nodes = append(nodes, entry)
			bytes += len(entry)
		}
	}
	return nodes
}

func handleGetReceipts66(backend Backend, msg Decoder, peer *Peer) error {
	// Decode the block receipts retrieval message
	var query GetReceiptsPacket66
	if err := msg.Decode(&query); err != nil {
		return fmt.Errorf("%w: message %v: %v", errDecode, msg, err)
	}
	response := ServiceGetReceiptsQuery(backend.Chain(), query.GetReceiptsPacket)
	return peer.ReplyReceiptsRLP(query.RequestId, response)
}

// ServiceGetReceiptsQuery assembles the response to a receipt query. It is
// exposed to allow external packages to test protocol behavior.
func ServiceGetReceiptsQuery(chain *core.BlockChain, query GetReceiptsPacket) []rlp.RawValue {
	// Gather state data until the fetch or network limits is reached
	var (
		bytes    int
		receipts []rlp.RawValue
	)
	for lookups, hash := range query {
		if bytes >= softResponseLimit || len(receipts) >= maxReceiptsServe ||
			lookups >= 2*maxReceiptsServe {
			break
		}
		// Retrieve the requested block's receipts
		results := chain.GetReceiptsByHash(hash)
		if results == nil {
			if header := chain.GetHeaderByHash(hash); header == nil || header.ReceiptHash != types.EmptyRootHash {
				continue
			}
		}
		// If known, encode and queue for response packet
		if encoded, err := rlp.EncodeToBytes(results); err != nil {
			log.Error("Failed to encode receipt", "err", err)
		} else {
			receipts = append(receipts, encoded)
			bytes += len(encoded)
		}
	}
	return receipts
}

func handleNewBlockhashes(backend Backend, msg Decoder, peer *Peer) error {
	// A batch of new block announcements just arrived
	ann := new(NewBlockHashesPacket)
	if err := msg.Decode(ann); err != nil {
		return fmt.Errorf("%w: message %v: %v", errDecode, msg, err)
	}
	// Mark the hashes as present at the remote node
	for _, block := range *ann {
		peer.markBlock(block.Hash)
	}
	// Deliver them all to the backend for queuing
	return backend.Handle(peer, ann)
}

func handleNewBlock(backend Backend, msg Decoder, peer *Peer) error {
	// Retrieve the propagated block, leaving its body encoded until we know
	// it's not stuffed with more transactions than its gas allows
	raw := new(rawNewBlockPacket)
	if err := msg.Decode(raw); err != nil {
		return fmt.Errorf("%w: message %v: %v", errDecode, msg, err)
	}
	header := raw.Block.Header
	content, _, err := rlp.SplitList(raw.Block.Txs)
	if err != nil {
		return fmt.Errorf("%w: message %v: transactions: %v", errDecode, msg, err)
	}
	txs, err := rlp.CountValues(content)
	if err != nil {
		return fmt.Errorf("%w: message %v: transactions: %v", errDecode, msg, err)
	}
	// Every transaction burns at least TxGas, so no valid block can hold more
	// than its gas used allows
	if header.GasUsed > header.GasLimit || uint64(txs) > header.GasUsed/params.TxGas {
		return fmt.Errorf("%w: block with %d transactions, gas used %d, gas limit %d", errDecode, txs, header.GasUsed, header.GasLimit)
	}
	// The gas limit only drifts slowly, so a block claiming far more room than
	// our chain has is either bogus or too far ahead to import. Skip it without
	// decoding the body, but don't punish the peer in case we're just behind.
	if limit := backend.Chain().CurrentBlock().GasLimit(); header.GasLimit > 2*limit {
		peer.Log().Debug("Ignoring propagated block with oversized gas limit", "number", header.Number, "hash", header.Hash(), "gaslimit", header.GasLimit, "local", limit)
		return nil
	}
	var body struct {
		Txs    []*types.Transaction
		Uncles []*types.Header
	}
	if err := rlp.DecodeBytes(raw.Block.Txs, &body.Txs); err != nil {
		return fmt.Errorf("%w: message %v: transactions: %v", errDecode, msg, err)
	}
	if err := rlp.DecodeBytes(raw.Block.Uncles, &body.Uncles); err != nil {
		return fmt.Errorf("%w: message %v: uncles: %v", errDecode, msg, err)
	}
	ann := &NewBlockPacket{
		Block: types.NewBlockWithHeader(header).WithBody(body.Txs, body.Uncles),
		TD:    raw.TD,
	}
	if err := ann.sanityCheck(); err != nil {
		return err
	}
	if hash := types.CalcUncleHash(ann.Block.Uncles()); hash != ann.Block.UncleHash() {
		log.Warn("Propagated block has invalid uncles", "have", hash, "exp", ann.Block.UncleHash())
		return nil // TODO(karalabe): return error eventually, but wait a few releases
	}
	if hash := types.DeriveSha(ann.Block.Transactions(), trie.NewStackTrie(nil)); hash != ann.Block.TxHash() {
		log.Warn("Propagated block has invalid body", "have", hash, "exp", ann.Block.TxHash())
		return nil // TODO(karalabe): return error eventually, but wait a few releases
	}
	ann.Block.ReceivedAt = msg.Time()
	ann.Block.ReceivedFrom = peer

	// Mark the peer as owning the block
	peer.markBlock(ann.Block.Hash())

	return backend.Handle(peer, ann)
}

func handleBlockHeaders66(backend Backend, msg Decoder, peer *Peer) error {
	// A batch of headers arrived to one of our previous requests
	res := new(rawResponsePacket66)
	if err := msg.Decode(res); err != nil {
		return fmt.Errorf("%w: message %v: %v", errDecode, msg, err)
	}
	var headers BlockHeadersPacket
	decode := func(req *Request) (interface{}, error) {
		if err := decodeLimitedList(res.Payload, requestedItems(req), &headers); err != nil {
			return nil, err
		}
		return &headers, nil
	}
	metadata := func() interface{} {
		hashes := make([]common.Hash, len(headers))
		for i, header := range headers {
			hashes[i] = header.Hash()
		}
		return hashes
	}
	return peer.dispatchResponse(&Response{
		id:   res.RequestId,
		code: BlockHeadersMsg,
	}, decode, metadata)
}

func handleBlockBodies66(backend Backend, msg Decoder, peer *Peer) error {
	// A batch of block bodies arrived to one of our previous requests
	res := new(rawResponsePacket66)
	if err := msg.Decode(res); err != nil {
		return fmt.Errorf("%w: message %v: %v", errDecode, msg, err)
	}
	// Bodies are only split into their transaction and uncle lists here, and
	// get decoded by the requester once their hashes match the headers.
	var bodies RawBlockBodiesPacket
	decode := func(req *Request) (interface{}, error) {
		if err := decodeLimitedList(res.Payload, requestedItems(req), &bodies); err != nil {
			return nil, err
		}
		for i := range bodies {
			if _, _, err := rlp.SplitList(bodies[i].Transactions); err != nil {
				return nil, fmt.Errorf("body %d transactions: %v", i, err)
			}
			if _, _, err := rlp.SplitList(bodies[i].Uncles); err != nil {
				return nil, fmt.Errorf("body %d uncles: %v", i, err)
			}
		}
		return &bodies, nil
	}
	metadata := func() interface{} {
		var (
			txsHashes   = make([]common.Hash, len(bodies))
			uncleHashes = make([]common.Hash, len(bodies))
		)
		hasher := trie.NewStackTrie(nil)
		for i, body := range bodies {
			// A list holding things that can't be transactions matches no
			// header, so leave its hash empty rather than hashing the junk.
			if txs, ok := newRawTypedList(body.Transactions, minTxContent); ok {
				txsHashes[i] = types.DeriveSha(txs, hasher)
			}
			if bytes.Equal(body.Uncles, emptyListRLP) {
				uncleHashes[i] = types.EmptyUncleHash
			} else {
				uncleHashes[i] = crypto.Keccak256Hash(body.Uncles)
			}
		}
		return [][]common.Hash{txsHashes, uncleHashes}
	}
	return peer.dispatchResponse(&Response{
		id:   res.RequestId,
		code: BlockBodiesMsg,
	}, decode, metadata)
}

// emptyListRLP is the encoding of an empty list.
var emptyListRLP = []byte{0xc0}

// Legacy items in a transaction or receipt list are RLP lists of at least nine
// and four fields respectively, the latter including a 256 byte bloom. Anything
// shorter can't be one, which saves hashing junk.
const (
	minTxContent      = 9
	minReceiptContent = 1 + 1 + 3 + 256 + 1
)

// rawTypedList implements types.DerivableList over an encoded list of
// transactions or receipts, so its root can be computed without decoding the
// items. Only a 4 byte offset is kept per item.
type rawTypedList struct {
	data    []byte
	offsets []uint32
}

// newRawTypedList indexes an encoded transaction or receipt list. It fails if
// any item can't be one: legacy items are lists of at least minContent bytes,
// typed ones a string holding a type byte and a list.
func newRawTypedList(list rlp.RawValue, minContent int) (*rawTypedList, bool) {
	content, _, err := rlp.SplitList(list)
	if err != nil {
		return nil, false
	}
	// Check every item before allocating anything for them
	var count int
	for rest := content; len(rest) > 0; count++ {
		kind, payload, next, err := rlp.Split(rest)
		if err != nil {
			return nil, false
		}
		switch kind {
		case rlp.List:
			if len(payload) < minContent {
				return nil, false
			}
		case rlp.String:
			if len(payload) < 2 || payload[0] >= 0x80 || payload[1] < 0xc0 {
				return nil, false
			}
		default:
			return nil, false
		}
		rest = next
	}
	l := &rawTypedList{data: content, offsets: make([]uint32, 0, count)}
	for rest := content; len(rest) > 0; {
		_, _, next, _ := rlp.Split(rest)
		l.offsets = append(l.offsets, uint32(len(content)-len(rest)))
		rest = next
	}
	return l, true
}

// Len returns the number of items in the list.
func (l *rawTypedList) Len() int {
	return len(l.offsets)
}

// EncodeIndex writes the i'th item the way it is hashed into a trie: legacy
// items as is, typed ones without the string wrapper.
func (l *rawTypedList) EncodeIndex(i int, buf *bytes.Buffer) {
	end := uint32(len(l.data))
	if i != len(l.offsets)-1 {
		end = l.offsets[i+1]
	}
	item := l.data[l.offsets[i]:end]
	if kind, content, _, _ := rlp.Split(item); kind == rlp.String {
		buf.Write(content)
	} else {
		buf.Write(item)
	}
}

func handleNodeData66(backend Backend, msg Decoder, peer *Peer) error {
	// A batch of node state data arrived to one of our previous requests
	res := new(rawResponsePacket66)
	if err := msg.Decode(res); err != nil {
		return fmt.Errorf("%w: message %v: %v", errDecode, msg, err)
	}
	decode := func(req *Request) (interface{}, error) {
		var data NodeDataPacket
		if err := decodeLimitedList(res.Payload, requestedItems(req), &data); err != nil {
			return nil, err
		}
		return &data, nil
	}
	return peer.dispatchResponse(&Response{
		id:   res.RequestId,
		code: NodeDataMsg,
	}, decode, nil) // No post-processing, we're not using this packet anymore
}

func handleReceipts66(backend Backend, msg Decoder, peer *Peer) error {
	// A batch of receipts arrived to one of our previous requests
	res := new(rawResponsePacket66)
	if err := msg.Decode(res); err != nil {
		return fmt.Errorf("%w: message %v: %v", errDecode, msg, err)
	}
	// Receipts stay encoded too, and get decoded by the requester once their
	// hashes match the headers.
	var receipts RawReceiptsPacket
	decode := func(req *Request) (interface{}, error) {
		if err := decodeLimitedList(res.Payload, requestedItems(req), &receipts); err != nil {
			return nil, err
		}
		for i := range receipts {
			if _, _, err := rlp.SplitList(receipts[i]); err != nil {
				return nil, fmt.Errorf("receipts %d: %v", i, err)
			}
		}
		return &receipts, nil
	}
	metadata := func() interface{} {
		hasher := trie.NewStackTrie(nil)
		hashes := make([]common.Hash, len(receipts))
		for i, receipt := range receipts {
			// Junk matches no header, leave its hash empty
			if list, ok := newRawTypedList(receipt, minReceiptContent); ok {
				hashes[i] = types.DeriveSha(list, hasher)
			}
		}
		return hashes
	}
	return peer.dispatchResponse(&Response{
		id:   res.RequestId,
		code: ReceiptsMsg,
	}, decode, metadata)
}

// rawNewBlockPacket is a NewBlockPacket with the block body left encoded.
type rawNewBlockPacket struct {
	Block struct {
		Header *types.Header
		Txs    rlp.RawValue
		Uncles rlp.RawValue
	}
	TD *big.Int
}

// rawResponsePacket66 is the eth/66 envelope of a response packet with the
// payload left undecoded, so it can be matched up to a request first.
type rawResponsePacket66 struct {
	RequestId uint64
	Payload   rlp.RawValue
}

// requestedItems returns how many items a response to req may hold at most.
func requestedItems(req *Request) int {
	switch data := req.data.(type) {
	case *GetBlockHeadersPacket66:
		if data.Amount > maxMessageSize {
			return maxMessageSize
		}
		return int(data.Amount)
	case *GetBlockBodiesPacket66:
		return len(data.GetBlockBodiesPacket)
	case *GetNodeDataPacket66:
		return len(data.GetNodeDataPacket)
	case *GetReceiptsPacket66:
		return len(data.GetReceiptsPacket)
	}
	return 0
}

// decodeLimitedList decodes an RLP list into val, refusing it without decoding
// if it holds more than limit items. Decoding a long list of tiny items blows
// up in memory far beyond its wire size, so the count is checked first.
func decodeLimitedList(raw rlp.RawValue, limit int, val interface{}) error {
	content, _, err := rlp.SplitList(raw)
	if err != nil {
		return err
	}
	count, err := rlp.CountValues(content)
	if err != nil {
		return err
	}
	if count > limit {
		return fmt.Errorf("%w: %d items, limit %d", errTooManyItems, count, limit)
	}
	return rlp.DecodeBytes(raw, val)
}

func handleNewPooledTransactionHashes(backend Backend, msg Decoder, peer *Peer) error {
	// New transaction announcement arrived, make sure we have
	// a valid and fresh chain to handle them
	if !backend.AcceptTxs() {
		return nil
	}
	ann := new(NewPooledTransactionHashesPacket)
	if err := msg.Decode(ann); err != nil {
		return fmt.Errorf("%w: message %v: %v", errDecode, msg, err)
	}
	// Schedule all the unknown hashes for retrieval
	for _, hash := range *ann {
		peer.markTransaction(hash)
	}
	return backend.Handle(peer, ann)
}

func handleGetPooledTransactions66(backend Backend, msg Decoder, peer *Peer) error {
	// Decode the pooled transactions retrieval message
	var query GetPooledTransactionsPacket66
	if err := msg.Decode(&query); err != nil {
		return fmt.Errorf("%w: message %v: %v", errDecode, msg, err)
	}
	hashes, txs := answerGetPooledTransactions(backend, query.GetPooledTransactionsPacket, peer)
	return peer.ReplyPooledTransactionsRLP(query.RequestId, hashes, txs)
}

func answerGetPooledTransactions(backend Backend, query GetPooledTransactionsPacket, peer *Peer) ([]common.Hash, []rlp.RawValue) {
	// Gather transactions until the fetch or network limits is reached
	var (
		bytes  int
		hashes []common.Hash
		txs    []rlp.RawValue
	)
	for _, hash := range query {
		if bytes >= softResponseLimit {
			break
		}
		// Retrieve the requested transaction, skipping if unknown to us
		tx := backend.TxPool().Get(hash)
		if tx == nil {
			continue
		}
		// If known, encode and queue for response packet
		if encoded, err := rlp.EncodeToBytes(tx); err != nil {
			log.Error("Failed to encode transaction", "err", err)
		} else {
			hashes = append(hashes, hash)
			txs = append(txs, encoded)
			bytes += len(encoded)
		}
	}
	return hashes, txs
}

func handleTransactions(backend Backend, msg Decoder, peer *Peer) error {
	// Transactions arrived, make sure we have a valid and fresh chain to handle them
	if !backend.AcceptTxs() {
		return nil
	}
	// Transactions can be processed, parse all of them and deliver to the pool
	var (
		raw rlp.RawValue
		txs TransactionsPacket
	)
	if err := msg.Decode(&raw); err != nil {
		return fmt.Errorf("%w: message %v: %v", errDecode, msg, err)
	}
	if err := decodeLimitedList(raw, maxTransactionsPerPacket, &txs); err != nil {
		return fmt.Errorf("%w: message %v: %v", errDecode, msg, err)
	}
	for i, tx := range txs {
		// Validate and mark the remote transaction
		if tx == nil {
			return fmt.Errorf("%w: transaction %d is nil", errDecode, i)
		}
		peer.markTransaction(tx.Hash())
	}
	return backend.Handle(peer, &txs)
}

func handlePooledTransactions66(backend Backend, msg Decoder, peer *Peer) error {
	// Transactions arrived, make sure we have a valid and fresh chain to handle them
	if !backend.AcceptTxs() {
		return nil
	}
	// Transactions can be processed, parse all of them and deliver to the pool
	var (
		res rawResponsePacket66
		txs PooledTransactionsPacket66
	)
	if err := msg.Decode(&res); err != nil {
		return fmt.Errorf("%w: message %v: %v", errDecode, msg, err)
	}
	txs.RequestId = res.RequestId
	if err := decodeLimitedList(res.Payload, maxTransactionsPerPacket, &txs.PooledTransactionsPacket); err != nil {
		return fmt.Errorf("%w: message %v: %v", errDecode, msg, err)
	}
	for i, tx := range txs.PooledTransactionsPacket {
		// Validate and mark the remote transaction
		if tx == nil {
			return fmt.Errorf("%w: transaction %d is nil", errDecode, i)
		}
		peer.markTransaction(tx.Hash())
	}
	requestTracker.Fulfil(peer.id, peer.version, PooledTransactionsMsg, txs.RequestId)

	return backend.Handle(peer, &txs.PooledTransactionsPacket)
}
