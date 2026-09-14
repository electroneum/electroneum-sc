// Copyright 2022 Electroneum Ltd
// This file is part of the electroneum-sc library.

package common

// Electroneum-specific additions to the common package.
//
// These live in their own file rather than in types.go so the ETN delta stays
// isolated from upstream go-ethereum, which keeps future rebases mechanical.
//
// PublicKey is the 65-byte uncompressed secp256k1 public key carried by the
// priority signature on a PriorityTx, and PriorityTransactorMap is the
// allowlist read from the on-chain priority-transactors contract.

import (
	"bytes"
	"database/sql/driver"
	"encoding/hex"
	"fmt"
	"math/big"
	"reflect"

	"github.com/electroneum/electroneum-sc/common/hexutil"
	"github.com/electroneum/electroneum-sc/crypto/secp256k1"
)

const PublicKeyLength = 65

var PublicKeyT = reflect.TypeOf(PublicKey{})

// EmptyHash reports whether h is the zero hash.
func EmptyHash(h Hash) bool { return h == Hash{} }

// StringToAddress and StringToHash are Istanbul dependencies.
func StringToAddress(s string) Address { return BytesToAddress([]byte(s)) }
func StringToHash(s string) Hash       { return BytesToHash([]byte(s)) }

type PriorityTransactor struct {
	IsGasPriceWaiver bool
	EntityName       string
}

// PublicKey represents the 65 byte *uncompressed* secp256k1 pubkey used for priority signatures within txes of PriorityTx type
type PublicKey [PublicKeyLength]byte

type PriorityTransactorMap map[PublicKey]PriorityTransactor

// IsValid checks if the public key is a valid uncompressed secp256k1 public key and if it lies on the secp256k1 curve
func (p PublicKey) IsValid() bool {
	if p[0] != 4 {
		// uncompressed public keys should start with 0x04
		return false
	}
	// checks if the points lie on the secp256k1 curve
	x := new(big.Int).SetBytes(p[1:33])
	y := new(big.Int).SetBytes(p[33:65])
	return secp256k1.S256().IsOnCurve(x, y)
}

// BytesToPublicKey returns PublicKey with value b.
// If b is larger than len(h), b will be cropped from the left.
func BytesToPublicKey(b []byte) PublicKey {
	var p PublicKey
	p.SetBytes(b)
	return p
}

func StringToPublicKey(s string) PublicKey { return BytesToPublicKey([]byte(s)) }

// BigToPublicKey returns PublicKey with byte values of b.
// If b is larger than len(h), b will be cropped from the left.
func BigToPublicKey(b *big.Int) PublicKey { return BytesToPublicKey(b.Bytes()) }

// HexToPublicKey returns PublicKey with byte values of s.
// If s is larger than len(h), s will be cropped from the left.
func HexToPublicKey(s string) PublicKey { return BytesToPublicKey(FromHex(s)) }

// IsHexPublicKey verifies whether a string can represent a valid hex-encoded
// secp256k1 PublicKey or not.
func IsHexPublicKey(s string) bool {
	if has0xPrefix(s) {
		s = s[2:]
	}
	return len(s) == 2*PublicKeyLength && isHex(s)
}

// Bytes gets the string representation of the underlying PublicKey.
func (p PublicKey) Bytes() []byte { return p[:] }

// Hash converts an PublicKey to a hash by left-padding it with zeros.
func (p PublicKey) Hash() Hash { return BytesToHash(p[:]) }

// String implements fmt.Stringer.
func (p PublicKey) String() string {
	return string(p.hex())
}

func (p PublicKey) ToHexString() string {
	return string(p.hex())
}

func (p PublicKey) ToUnprefixedHexString() string {
	return string(p.hex()[2:])
}

func (p PublicKey) hex() []byte {
	var buf [len(p)*2 + 2]byte
	copy(buf[:2], "0x")
	hex.Encode(buf[2:], p[:])
	return buf[:]
}

// Format implements fmt.Formatter.
// PublicKey supports the %v, %s, %q, %x, %X and %d format verbs.
func (p PublicKey) Format(s fmt.State, c rune) {
	switch c {
	case 'v', 's', 'x', 'X':
		hex := p.hex()
		if !s.Flag('#') {
			hex = hex[2:]
		}
		if c == 'X' {
			hex = bytes.ToUpper(hex)
		}
		s.Write(hex)
	case 'q':
		q := []byte{'"'}
		s.Write(q)
		s.Write(p.hex())
		s.Write(q)
	case 'd':
		fmt.Fprint(s, ([len(p)]byte)(p))
	default:
		fmt.Fprintf(s, "%%!%c(PublicKey=%x)", c, p)
	}
}

// SetBytes sets the PublicKey to the value of b.
// If b is larger than len(a), b will be cropped from the left.
func (p *PublicKey) SetBytes(b []byte) {
	if len(b) > len(p) {
		b = b[len(b)-PublicKeyLength:]
	}
	copy(p[PublicKeyLength-len(b):], b)
}

// MarshalText returns the hex representation of a.
func (p PublicKey) MarshalText() ([]byte, error) {
	return hexutil.Bytes(p[:]).MarshalText()
}

// UnmarshalText parses a hash in hex syntax.
func (p *PublicKey) UnmarshalText(input []byte) error {
	return hexutil.UnmarshalFixedText("PublicKey", input, p[:])
}

// UnmarshalJSON parses a hash in hex syntax.
func (p *PublicKey) UnmarshalJSON(input []byte) error {
	return hexutil.UnmarshalFixedJSON(PublicKeyT, input, p[:])
}

// Scan implements Scanner for database/sql.
func (p *PublicKey) Scan(src interface{}) error {
	srcB, ok := src.([]byte)
	if !ok {
		return fmt.Errorf("can't scan %T into PublicKey", src)
	}
	if len(srcB) != PublicKeyLength {
		return fmt.Errorf("can't scan []byte of len %d into PublicKey, want %d", len(srcB), PublicKeyLength)
	}
	copy(p[:], srcB)
	return nil
}

// Value implements valuer for database/sql.
func (p PublicKey) Value() (driver.Value, error) {
	return p[:], nil
}

// ImplementsGraphQLType returns true if Hash implements the specified GraphQL type.
func (p PublicKey) ImplementsGraphQLType(name string) bool { return name == "PublicKey" }

// UnmarshalGraphQL unmarshals the provided GraphQL query data.
func (p *PublicKey) UnmarshalGraphQL(input interface{}) error {
	var err error
	switch input := input.(type) {
	case string:
		err = p.UnmarshalText([]byte(input))
	default:
		err = fmt.Errorf("unexpected type %T for PublicKey", input)
	}
	return err
}

// UnprefixedPublicKey allows marshaling an PublicKey without 0x prefix.
type UnprefixedPublicKey PublicKey

// UnmarshalText decodes the address from hex. The 0x prefix is optional.
func (p *UnprefixedPublicKey) UnmarshalText(input []byte) error {
	return hexutil.UnmarshalFixedUnprefixedText("UnprefixedPublicKey", input, p[:])
}

// MarshalText encodes the address as hex.
func (p UnprefixedPublicKey) MarshalText() ([]byte, error) {
	return []byte(hex.EncodeToString(p[:])), nil
}
