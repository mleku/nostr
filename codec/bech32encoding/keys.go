package bech32encoding

import (
	btcec "ec.mleku.dev/v2"
	"ec.mleku.dev/v2/bech32"
	"ec.mleku.dev/v2/schnorr"
	"ec.mleku.dev/v2/secp256k1"
	. "nostr.mleku.dev"
	"util.mleku.dev/hex"
)

const (
	// MinKeyStringLen is 56 because Bech32 needs 52 characters plus 4 for the HRP,
	// any string shorter than this cannot be a nostr key.
	MinKeyStringLen = 56
	HexKeyLen       = 64
	Bech32HRPLen    = 4
)

var (
	SecHRP = B("nsec")
	PubHRP = B("npub")
)

// ConvertForBech32 performs the bit expansion required for encoding into Bech32.
func ConvertForBech32(b8 B) (b5 B, err error) { return bech32.ConvertBits(b8, 8, 5, true) }

// ConvertFromBech32 collapses together the bit expanded 5 bit numbers encoded in bech32.
func ConvertFromBech32(b5 B) (b8 B, err error) { return bech32.ConvertBits(b5, 5, 8, true) }

// SecretKeyToNsec encodes an secp256k1 secret key as a Bech32 string (nsec).
func SecretKeyToNsec(sk *secp256k1.SecretKey) (encoded B, err error) {
	var b5 B
	if b5, err = ConvertForBech32(sk.Serialize()); Chk.E(err) {
		return
	}
	return bech32.Encode(SecHRP, b5)
}

// PublicKeyToNpub encodes a public key as a bech32 string (npub).
func PublicKeyToNpub(pk *secp256k1.PublicKey) (encoded B, err error) {
	var bits5 B
	pubKeyBytes := schnorr.SerializePubKey(pk)
	if bits5, err = ConvertForBech32(pubKeyBytes); Chk.E(err) {
		return
	}
	return bech32.Encode(PubHRP, bits5)
}

// NsecToSecretKey decodes a nostr secret key (nsec) and returns the secp256k1
// secret key.
func NsecToSecretKey(encoded B) (sk *secp256k1.SecretKey, err error) {
	var b5, b8, hrp B
	if hrp, b5, err = bech32.Decode(encoded); Chk.E(err) {
		return
	}
	if !Equals(hrp, SecHRP) {
		err = Log.E.Err("wrong human readable part, got '%s' want '%s'",
			hrp, SecHRP)
		return
	}
	if b8, err = ConvertFromBech32(b5); Chk.E(err) {
		return
	}
	sk = secp256k1.SecKeyFromBytes(b8)
	return
}

// NpubToPublicKey decodes an nostr public key (npub) and returns an secp256k1
// public key.
func NpubToPublicKey(encoded B) (pk *secp256k1.PublicKey, err error) {
	var b5, b8, hrp B
	if hrp, b5, err = bech32.Decode(encoded); Chk.E(err) {
		err = Log.E.Err("ERROR: '%s'", err)
		return
	}
	if !Equals(hrp, PubHRP) {
		err = Log.E.Err("wrong human readable part, got '%s' want '%s'",
			hrp, PubHRP)
		return
	}
	if b8, err = ConvertFromBech32(b5); Chk.E(err) {
		return
	}

	return schnorr.ParsePubKey(b8[:32])
}

// HexToPublicKey decodes a string that should be a 64 character long hex
// encoded public key into a btcec.PublicKey that can be used to verify a
// signature or encode to Bech32.
func HexToPublicKey(pk string) (p *btcec.PublicKey, err error) {
	if len(pk) != HexKeyLen {
		err = Log.E.Err("secret key is %d bytes, must be %d", len(pk),
			HexKeyLen)
		return
	}
	var pb B
	if pb, err = hex.Dec(pk); Chk.D(err) {
		return
	}
	if p, err = schnorr.ParsePubKey(pb); Chk.D(err) {
		return
	}
	return
}

// HexToSecretKey decodes a string that should be a 64 character long hex
// encoded public key into a btcec.PublicKey that can be used to verify a
// signature or encode to Bech32.
func HexToSecretKey(sk B) (s *btcec.SecretKey, err error) {
	if len(sk) != HexKeyLen {
		err = Log.E.Err("secret key is %d bytes, must be %d", len(sk),
			HexKeyLen)
		return
	}
	pb := make(B, schnorr.PubKeyBytesLen)
	if _, err = hex.DecBytes(pb, sk); Chk.D(err) {
		return
	}
	if s = secp256k1.SecKeyFromBytes(pb); Chk.D(err) {
		return
	}
	return
}

func HexToNpub(publicKeyHex B) (s B, err error) {
	b := make(B, schnorr.PubKeyBytesLen)
	if _, err = hex.DecBytes(b, publicKeyHex); Chk.D(err) {
		err = Log.E.Err("failed to decode public key hex: %w", err)
		return
	}
	var bits5 B
	if bits5, err = bech32.ConvertBits(b, 8, 5, true); Chk.D(err) {
		return nil, err
	}
	return bech32.Encode(NpubHRP, bits5)
}

func BinToNpub(b B) (s B, err error) {
	var bits5 B
	if bits5, err = bech32.ConvertBits(b, 8, 5, true); Chk.D(err) {
		return nil, err
	}
	return bech32.Encode(NpubHRP, bits5)
}

// HexToNsec converts a hex encoded secret key to a bech32 encoded nsec.
func HexToNsec(sk B) (nsec B, err error) {
	var s *btcec.SecretKey
	if s, err = HexToSecretKey(sk); Chk.E(err) {
		return
	}
	if nsec, err = SecretKeyToNsec(s); Chk.E(err) {
		return
	}
	return
}

// BinToNsec converts a binary secret key to a bech32 encoded nsec.
func BinToNsec(sk B) (nsec B, err error) {
	var s *btcec.SecretKey
	s, _ = btcec.SecKeyFromBytes(sk)
	if nsec, err = SecretKeyToNsec(s); Chk.E(err) {
		return
	}
	return
}

// SecretKeyToHex converts a secret key to the hex encoding.
func SecretKeyToHex(sk *btcec.SecretKey) (hexSec B) {
	hex.EncBytes(hexSec, sk.Serialize())
	return
}

func NsecToHex(nsec B) (hexSec B, err error) {
	var sk *secp256k1.SecretKey
	if sk, err = NsecToSecretKey(nsec); Chk.E(err) {
		return
	}
	hexSec = SecretKeyToHex(sk)
	return
}
