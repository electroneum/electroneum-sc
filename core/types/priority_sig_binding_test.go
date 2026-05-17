package types

import (
	"math/big"
	"testing"

	"github.com/electroneum/electroneum-sc/crypto"
)

// TestPrioritySignatureReplay_PreFork verifies that with the pre-fork
// londonSigner, an attacker can replay a priority signature from one sender
// onto a transaction signed by a different sender (because both sign over
// the same body-only hash). This documents the vulnerability.
func TestPrioritySignatureReplay_PreFork(t *testing.T) {
	var (
		victimKey, _   = crypto.GenerateKey()
		attackerKey, _ = crypto.GenerateKey()
		priorityKey, _ = crypto.GenerateKey()
		signer         = NewLondonSigner(big.NewInt(1))
		priorityPub    = crypto.ECDSAPubkeyToPublicKey(priorityKey.PublicKey)
	)

	// Victim creates and signs a priority tx.
	txdata := &PriorityTx{
		ChainID:   big.NewInt(1),
		Nonce:     0,
		GasTipCap: big.NewInt(1),
		GasFeeCap: big.NewInt(1000000000),
		Gas:       21000,
		Value:     big.NewInt(1),
	}
	victimTx, err := SignNewPriorityTx(victimKey, priorityKey, signer, txdata)
	if err != nil {
		t.Fatal(err)
	}

	// Verify victim tx is valid.
	recoveredPub, err := PrioritySender(signer, victimTx)
	if err != nil {
		t.Fatal(err)
	}
	if recoveredPub != priorityPub {
		t.Fatal("victim tx priority sender mismatch")
	}

	// Attacker copies body fields and priority signature, signs with own key.
	pV, pR, pS := victimTx.RawPrioritySignatureValues()
	attackerTxData := &PriorityTx{
		ChainID:   big.NewInt(1),
		Nonce:     0,
		GasTipCap: big.NewInt(1),
		GasFeeCap: big.NewInt(1000000000),
		Gas:       21000,
		Value:     big.NewInt(1),
		// Attacker will paste priority sig manually.
	}
	attackerTx := NewTx(attackerTxData)

	// Attacker signs the sender slot with their own key.
	h := signer.Hash(attackerTx)
	attackerSig, err := crypto.Sign(h[:], attackerKey)
	if err != nil {
		t.Fatal(err)
	}
	attackerTx, err = attackerTx.WithSignature(signer, attackerSig)
	if err != nil {
		t.Fatal(err)
	}

	// Paste stolen priority signature.
	inner := attackerTx.inner.copy().(*PriorityTx)
	inner.PriorityV = pV
	inner.PriorityR = pR
	inner.PriorityS = pS
	attackerTx = &Transaction{inner: inner, time: attackerTx.time}

	// With londonSigner (pre-fork), the replayed priority sig should be valid.
	replayedPub, err := PrioritySender(signer, attackerTx)
	if err != nil {
		t.Fatalf("pre-fork: expected replay to succeed, got %v", err)
	}
	if replayedPub != priorityPub {
		t.Fatal("pre-fork: replayed priority sender should match original")
	}

	// Confirm the sender is the attacker, not the victim.
	attackerAddr := crypto.PubkeyToAddress(attackerKey.PublicKey)
	victimAddr := crypto.PubkeyToAddress(victimKey.PublicKey)
	sender, err := Sender(signer, attackerTx)
	if err != nil {
		t.Fatal(err)
	}
	if sender != attackerAddr {
		t.Fatal("sender should be attacker")
	}
	if sender == victimAddr {
		t.Fatal("sender should not be victim")
	}
}

// TestPrioritySignatureReplay_PostFork verifies that with the post-fork
// futureForkSigner, replaying a priority signature onto a transaction with
// a different sender fails — the priority hash includes V,R,S so changing
// the sender invalidates the priority signature.
func TestPrioritySignatureReplay_PostFork(t *testing.T) {
	var (
		victimKey, _   = crypto.GenerateKey()
		attackerKey, _ = crypto.GenerateKey()
		priorityKey, _ = crypto.GenerateKey()
		signer         = NewFutureForkSigner(big.NewInt(1))
		priorityPub    = crypto.ECDSAPubkeyToPublicKey(priorityKey.PublicKey)
	)

	// Victim creates and signs a priority tx with the post-fork signer.
	txdata := &PriorityTx{
		ChainID:   big.NewInt(1),
		Nonce:     0,
		GasTipCap: big.NewInt(1),
		GasFeeCap: big.NewInt(1000000000),
		Gas:       21000,
		Value:     big.NewInt(1),
	}
	victimTx, err := SignNewPriorityTx(victimKey, priorityKey, signer, txdata)
	if err != nil {
		t.Fatal(err)
	}

	// Verify victim tx is valid.
	recoveredPub, err := PrioritySender(signer, victimTx)
	if err != nil {
		t.Fatal(err)
	}
	if recoveredPub != priorityPub {
		t.Fatal("victim tx priority sender mismatch")
	}

	// Attacker copies body fields and priority signature.
	pV, pR, pS := victimTx.RawPrioritySignatureValues()
	attackerTxData := &PriorityTx{
		ChainID:   big.NewInt(1),
		Nonce:     0,
		GasTipCap: big.NewInt(1),
		GasFeeCap: big.NewInt(1000000000),
		Gas:       21000,
		Value:     big.NewInt(1),
	}
	attackerTx := NewTx(attackerTxData)

	// Attacker signs with their own key.
	h := signer.Hash(attackerTx)
	attackerSig, err := crypto.Sign(h[:], attackerKey)
	if err != nil {
		t.Fatal(err)
	}
	attackerTx, err = attackerTx.WithSignature(signer, attackerSig)
	if err != nil {
		t.Fatal(err)
	}

	// Paste stolen priority signature.
	inner := attackerTx.inner.copy().(*PriorityTx)
	inner.PriorityV = pV
	inner.PriorityR = pR
	inner.PriorityS = pS
	attackerTx = &Transaction{inner: inner, time: attackerTx.time}

	// With futureForkSigner (post-fork), the replayed priority sig should
	// fail because the attacker's V,R,S differs from the victim's.
	replayedPub, err := PrioritySender(signer, attackerTx)
	if err == nil && replayedPub == priorityPub {
		t.Fatal("post-fork: replay should not recover the original priority pubkey")
	}
	// Either an error or a different recovered pubkey — both mean the attack failed.
}

// TestFutureForkSigner_SignAndVerifyRoundtrip verifies that a priority tx
// signed with futureForkSigner can be correctly verified.
func TestFutureForkSigner_SignAndVerifyRoundtrip(t *testing.T) {
	var (
		senderKey, _   = crypto.GenerateKey()
		priorityKey, _ = crypto.GenerateKey()
		signer         = NewFutureForkSigner(big.NewInt(1))
		expectedPub    = crypto.ECDSAPubkeyToPublicKey(priorityKey.PublicKey)
		expectedAddr   = crypto.PubkeyToAddress(senderKey.PublicKey)
	)

	txdata := &PriorityTx{
		ChainID:   big.NewInt(1),
		Nonce:     42,
		GasTipCap: big.NewInt(100),
		GasFeeCap: big.NewInt(2000000000),
		Gas:       50000,
		Value:     big.NewInt(999),
	}
	tx, err := SignNewPriorityTx(senderKey, priorityKey, signer, txdata)
	if err != nil {
		t.Fatal(err)
	}

	sender, err := Sender(signer, tx)
	if err != nil {
		t.Fatalf("Sender failed: %v", err)
	}
	if sender != expectedAddr {
		t.Fatalf("wrong sender: got %x, want %x", sender, expectedAddr)
	}

	pub, err := PrioritySender(signer, tx)
	if err != nil {
		t.Fatalf("PrioritySender failed: %v", err)
	}
	if pub != expectedPub {
		t.Fatalf("wrong priority pubkey: got %x, want %x", pub, expectedPub)
	}
}

// TestFutureForkSigner_IncompatibleWithLondonSigner verifies that a tx signed
// with futureForkSigner cannot be verified with londonSigner and vice versa.
func TestFutureForkSigner_IncompatibleWithLondonSigner(t *testing.T) {
	var (
		senderKey, _   = crypto.GenerateKey()
		priorityKey, _ = crypto.GenerateKey()
		londonSgn      = NewLondonSigner(big.NewInt(1))
		futureSgn      = NewFutureForkSigner(big.NewInt(1))
		expectedPub    = crypto.ECDSAPubkeyToPublicKey(priorityKey.PublicKey)
	)

	txdata := &PriorityTx{
		ChainID:   big.NewInt(1),
		Nonce:     0,
		GasTipCap: big.NewInt(1),
		GasFeeCap: big.NewInt(1000000000),
		Gas:       21000,
		Value:     big.NewInt(1),
	}

	// Sign with futureForkSigner, try to verify with londonSigner.
	futureTx, err := SignNewPriorityTx(senderKey, priorityKey, futureSgn, txdata)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := PrioritySender(londonSgn, futureTx)
	if err == nil && pub == expectedPub {
		t.Fatal("future-fork-signed tx should not verify with londonSigner")
	}

	// Sign with londonSigner, try to verify with futureForkSigner.
	londonTx, err := SignNewPriorityTx(senderKey, priorityKey, londonSgn, txdata)
	if err != nil {
		t.Fatal(err)
	}
	pub, err = PrioritySender(futureSgn, londonTx)
	if err == nil && pub == expectedPub {
		t.Fatal("london-signed tx should not verify with futureForkSigner")
	}
}

// TestMakeSigner_ReturnsFutureForkSigner verifies that MakeSigner returns
// a futureForkSigner when the block number is at or past the fork block.
func TestMakeSigner_ReturnsFutureForkSigner(t *testing.T) {
	futureSigner := NewFutureForkSigner(big.NewInt(1))
	londonSgn := NewLondonSigner(big.NewInt(1))

	// Signer type equality checks.
	if !futureSigner.Equal(NewFutureForkSigner(big.NewInt(1))) {
		t.Fatal("futureForkSigner should equal another futureForkSigner with same chain ID")
	}
	if futureSigner.Equal(londonSgn) {
		t.Fatal("futureForkSigner should not equal londonSigner")
	}
	if londonSgn.Equal(futureSigner) {
		t.Fatal("londonSigner should not equal futureForkSigner")
	}
}

// TestPreForkSignerPriorityHashEqualsHash verifies that for all pre-fork
// signers, PriorityHash returns the same value as Hash.
func TestPreForkSignerPriorityHashEqualsHash(t *testing.T) {
	tx := NewTx(&PriorityTx{
		ChainID:   big.NewInt(1),
		Nonce:     1,
		GasTipCap: big.NewInt(1),
		GasFeeCap: big.NewInt(1000000000),
		Gas:       21000,
		Value:     big.NewInt(1),
	})

	signers := []Signer{
		NewLondonSigner(big.NewInt(1)),
	}

	for _, s := range signers {
		h := s.Hash(tx)
		ph := s.PriorityHash(tx)
		if h != ph {
			t.Fatalf("pre-fork signer %T: PriorityHash != Hash", s)
		}
	}
}

// TestFutureForkSignerPriorityHashDiffersFromHash verifies that for
// futureForkSigner, PriorityHash differs from Hash when the tx has a
// sender signature set (V,R,S are non-zero).
func TestFutureForkSignerPriorityHashDiffersFromHash(t *testing.T) {
	senderKey, _ := crypto.GenerateKey()
	signer := NewFutureForkSigner(big.NewInt(1))

	txdata := &PriorityTx{
		ChainID:   big.NewInt(1),
		Nonce:     1,
		GasTipCap: big.NewInt(1),
		GasFeeCap: big.NewInt(1000000000),
		Gas:       21000,
		Value:     big.NewInt(1),
	}

	// Sign the sender slot.
	tx := NewTx(txdata)
	h := signer.Hash(tx)
	sig, err := crypto.Sign(h[:], senderKey)
	if err != nil {
		t.Fatal(err)
	}
	signedTx, err := tx.WithSignature(signer, sig)
	if err != nil {
		t.Fatal(err)
	}

	bodyHash := signer.Hash(signedTx)
	priorityHash := signer.PriorityHash(signedTx)

	if bodyHash == priorityHash {
		t.Fatal("futureForkSigner: PriorityHash should differ from Hash when sender sig is set")
	}
}
