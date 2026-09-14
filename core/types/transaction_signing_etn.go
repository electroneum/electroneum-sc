// Copyright 2022 Electroneum Ltd
// This file is part of the electroneum-sc library.

package types

// Priority-transaction signing. A PriorityTx carries a SECOND signature, made
// by a key on the on-chain priority-transactor allowlist, over the same body
// hash the sender signs.
//
// PrioritySender is deliberately NOT added to the Signer interface: only the
// London signer is ever selected on Electroneum (London at block 0, no Cancun),
// so widening the interface would force empty stubs onto every other signer for
// no benefit.

import (
	"crypto/ecdsa"
	"errors"
	"math/big"

	"github.com/electroneum/electroneum-sc/common"
	"github.com/electroneum/electroneum-sc/crypto"
)

// ErrTxIsNotPriorityType is returned when a priority operation is attempted on
// an ordinary transaction.
var ErrTxIsNotPriorityType = errors.New("tx is not priority transaction")

// prioritySigner is implemented by signers that understand PriorityTx. Kept
// separate from Signer so the other signers need no empty stubs.
type prioritySigner interface {
	PrioritySender(tx *Transaction) (common.PublicKey, error)
	PriorityHash(tx *Transaction) common.Hash
}

// priorityHash returns the hash the priority key signs over.
func priorityHash(s Signer, tx *Transaction) (common.Hash, error) {
	ps, ok := s.(prioritySigner)
	if !ok {
		return common.Hash{}, ErrTxTypeNotSupported
	}
	return ps.PriorityHash(tx), nil
}

// PrioritySender recovers the priority public key that signed tx, verifying the
// signature in the process.
func PrioritySender(signer Signer, tx *Transaction) (common.PublicKey, error) {
	if tx.Type() != PriorityTxType {
		return common.PublicKey{}, ErrTxIsNotPriorityType
	}
	if pk, ok := tx.priorityPubkey.Load().(common.PublicKey); ok {
		return pk, nil
	}
	ps, ok := signer.(prioritySigner)
	if !ok {
		return common.PublicKey{}, ErrTxTypeNotSupported
	}
	pk, err := ps.PrioritySender(tx)
	if err != nil {
		return common.PublicKey{}, err
	}
	tx.priorityPubkey.Store(pk)
	return pk, nil
}

// PriorityHash for londonSigner (pre-FutureFork) is the same as Hash: both the
// sender and the priority key sign over the body-only hash. After FutureFork,
// futureForkSigner binds the priority signature to the sender's V,R,S instead.
func (s londonSigner) PriorityHash(tx *Transaction) common.Hash {
	return s.Hash(tx)
}

func (s londonSigner) PrioritySender(tx *Transaction) (common.PublicKey, error) { //this will actually verify the sig too, as the sender() function does for the regular tx sigs
	switch inner := tx.inner.(type) {
	case *PriorityTx:
		V, R, S := inner.rawPrioritySignatureValues()
		// DynamicFee txs are defined to use 0 and 1 as their recovery
		// id, add 27 to become equivalent to unprotected Homestead signatures.
		V = new(big.Int).Add(V, big.NewInt(27))
		if tx.ChainId().Cmp(s.chainId) != 0 {
			return common.PublicKey{}, ErrInvalidChainId
		}
		return recoverPublicKey(s.Hash(tx), V, R, S, true)
	default:
		return common.PublicKey{}, ErrTxTypeNotSupported
	}
}

func recoverPublicKey(sighash common.Hash, Vb, R, S *big.Int, homestead bool) (common.PublicKey, error) {
	if Vb.BitLen() > 8 {
		return common.PublicKey{}, ErrInvalidSig
	}
	V := byte(Vb.Uint64() - 27)
	if !crypto.ValidateSignatureValues(V, R, S, homestead) {
		return common.PublicKey{}, ErrInvalidSig
	}
	// encode the signature in uncompressed format
	r, s := R.Bytes(), S.Bytes()
	sig := make([]byte, crypto.SignatureLength)
	copy(sig[32-len(r):32], r)
	copy(sig[64-len(s):64], s)
	sig[64] = V
	// recover the public key from the signature
	pub, err := crypto.Ecrecover(sighash[:], sig)
	if err != nil {
		return common.PublicKey{}, err
	}
	if len(pub) == 0 || pub[0] != 4 {
		return common.PublicKey{}, errors.New("invalid public key")
	}
	var pubkey common.PublicKey
	copy(pubkey[:], pub[:65])
	return pubkey, nil
}

// --- priority signing ---------------------------------------------------
//
// Needed to CREATE priority transactions. A follower node never signs one,
// but ETN's own priority-signature tests do, and futureForkSigner carries the
// FutureForkBlock gate that a fork must reproduce exactly.

// SignPriorityTx signs the transaction using the given signer and private key.
// The sender signs first over Hash(tx), then the priority key signs over
// PriorityHash(tx) which includes the sender's V,R,S — binding the priority
// signature to a specific sender.
func SignPriorityTx(tx *Transaction, s Signer, prv *ecdsa.PrivateKey, priorityPrv *ecdsa.PrivateKey) (*Transaction, error) {
	if tx.Type() != PriorityTxType {
		return nil, ErrTxIsNotPriorityType
	}
	h := s.Hash(tx)
	sig, err := crypto.Sign(h[:], prv)
	if err != nil {
		return nil, err
	}
	txCpy, err := tx.WithSignature(s, sig)
	if err != nil {
		return nil, err
	}
	ph := mustPriorityHash(s, txCpy)
	prioritySig, err := crypto.Sign(ph[:], priorityPrv)
	if err != nil {
		return nil, err
	}
	return txCpy.WithPrioritySignature(s, prioritySig)
}

// SignNewPriorityTx creates a priority transaction and signs it.
// The sender signs first, then the priority key signs over PriorityHash
// which includes the sender's signature.
func SignNewPriorityTx(prv *ecdsa.PrivateKey, priorityPrv *ecdsa.PrivateKey, s Signer, txdata TxData) (*Transaction, error) {
	tx := NewTx(txdata)
	if tx.Type() != PriorityTxType {
		return nil, ErrTxIsNotPriorityType
	}
	h := s.Hash(tx)
	sig, err := crypto.Sign(h[:], prv)
	if err != nil {
		return nil, err
	}
	txCpy, err := tx.WithSignature(s, sig)
	if err != nil {
		return nil, err
	}
	ph := mustPriorityHash(s, txCpy)
	prioritySig, err := crypto.Sign(ph[:], priorityPrv)
	if err != nil {
		return nil, err
	}
	return txCpy.WithPrioritySignature(s, prioritySig)
}

type futureForkSigner struct{ londonSigner }

// NewFutureForkSigner returns a signer that uses the sender-bound priority
// signature scheme introduced by the future fork.
func NewFutureForkSigner(chainId *big.Int) Signer {
	return futureForkSigner{londonSigner{eip2930Signer{NewEIP155Signer(chainId)}}}
}

func (s futureForkSigner) Sender(tx *Transaction) (common.Address, error) {
	if tx.Type() != DynamicFeeTxType && tx.Type() != PriorityTxType {
		return s.londonSigner.eip2930Signer.Sender(tx)
	}
	V, R, S := tx.RawSignatureValues()
	V = new(big.Int).Add(V, big.NewInt(27))
	if tx.ChainId().Cmp(s.chainId) != 0 {
		return common.Address{}, ErrInvalidChainId
	}
	if tx.Type() == PriorityTxType {
		_, err := s.PrioritySender(tx)
		if err != nil {
			return common.Address{}, err
		}
	}
	return recoverPlain(s.Hash(tx), R, S, V, true)
}

// PriorityHash returns a hash that includes the sender's signature (V, R, S)
// in addition to the transaction body fields. This binds the priority signature
// to the sender so it cannot be replayed by a different account.
func (s futureForkSigner) PriorityHash(tx *Transaction) common.Hash {
	if tx.Type() != PriorityTxType {
		return s.londonSigner.Hash(tx)
	}
	V, R, S := tx.RawSignatureValues()
	return prefixedRlpHash(
		tx.Type(),
		[]interface{}{
			s.chainId,
			tx.Nonce(),
			tx.GasTipCap(),
			tx.GasFeeCap(),
			tx.Gas(),
			tx.To(),
			tx.Value(),
			tx.Data(),
			tx.AccessList(),
			V, R, S,
		})
}

// PrioritySender recovers the priority public key using the sender-bound hash.
func (s futureForkSigner) PrioritySender(tx *Transaction) (common.PublicKey, error) {
	switch inner := tx.inner.(type) {
	case *PriorityTx:
		V, R, S := inner.rawPrioritySignatureValues()
		V = new(big.Int).Add(V, big.NewInt(27))
		if tx.ChainId().Cmp(s.chainId) != 0 {
			return common.PublicKey{}, ErrInvalidChainId
		}
		return recoverPublicKey(mustPriorityHash(s, tx), V, R, S, true)
	default:
		return common.PublicKey{}, ErrTxTypeNotSupported
	}
}

func (s futureForkSigner) Equal(s2 Signer) bool {
	x, ok := s2.(futureForkSigner)
	return ok && x.chainId.Cmp(s.chainId) == 0
}

// mustPriorityHash is priorityHash for the signing paths, which are only
// reachable once the signer is known to understand priority transactions.
func mustPriorityHash(s Signer, tx *Transaction) common.Hash {
	h, err := priorityHash(s, tx)
	if err != nil {
		panic(err)
	}
	return h
}

// Compile-time proof that the signers Electroneum actually selects implement
// the priority interface. The assertion in priorityHash is a runtime type
// switch, so without these a missing method would only surface as a failure
// while importing a block.
var (
	_ prioritySigner = londonSigner{}
	_ prioritySigner = futureForkSigner{}
)

// The signers below predate Electroneum's priority transaction and can never
// see one: MakeSigner only returns them for chains without London, and every
// Electroneum network enables London at block 0. They implement the interface
// so Signer can carry the priority methods, and refuse rather than guess.

func (s eip2930Signer) PrioritySender(tx *Transaction) (common.PublicKey, error) {
	return common.PublicKey{}, ErrTxTypeNotSupported
}
func (s eip2930Signer) PriorityHash(tx *Transaction) common.Hash { return s.Hash(tx) }

func (s EIP155Signer) PrioritySender(tx *Transaction) (common.PublicKey, error) {
	return common.PublicKey{}, ErrTxTypeNotSupported
}
func (s EIP155Signer) PriorityHash(tx *Transaction) common.Hash { return s.Hash(tx) }

func (hs HomesteadSigner) PrioritySender(tx *Transaction) (common.PublicKey, error) {
	return common.PublicKey{}, ErrTxTypeNotSupported
}
func (hs HomesteadSigner) PriorityHash(tx *Transaction) common.Hash { return hs.Hash(tx) }

func (fs FrontierSigner) PrioritySender(tx *Transaction) (common.PublicKey, error) {
	return common.PublicKey{}, ErrTxTypeNotSupported
}
func (fs FrontierSigner) PriorityHash(tx *Transaction) common.Hash { return fs.Hash(tx) }
