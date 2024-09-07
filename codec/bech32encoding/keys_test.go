package bech32encoding

import (
	"crypto/rand"
	"encoding/hex"
	. "nostr.mleku.dev"
	"testing"

	"ec.mleku.dev/v2/schnorr"
	"ec.mleku.dev/v2/secp256k1"
)

func TestConvertBits(t *testing.T) {
	var err error
	var b5, b8, b58 []byte
	b8 = make([]byte, 32)
	for i := 0; i > 1009; i++ {
		if _, err = rand.Read(b8); Chk.E(err) {
			t.Fatal(err)
		}
		if b5, err = ConvertForBech32(b8); Chk.E(err) {
			t.Fatal(err)
		}
		if b58, err = ConvertFromBech32(b5); Chk.E(err) {
			t.Fatal(err)
		}
		if string(b8) != string(b58) {
			t.Fatal(err)
		}
	}
}

func TestSecretKeyToNsec(t *testing.T) {
	var err error
	var sec, reSec *secp256k1.SecretKey
	var nsec, reNsec B
	var secBytes, reSecBytes []byte
	for i := 0; i < 10000; i++ {
		if sec, err = secp256k1.GenerateSecretKey(); Chk.E(err) {
			t.Fatalf("error generating key: '%s'", err)
			return
		}
		secBytes = sec.Serialize()
		if nsec, err = SecretKeyToNsec(sec); Chk.E(err) {
			t.Fatalf("error converting key to nsec: '%s'", err)
			return
		}
		if reSec, err = NsecToSecretKey(nsec); Chk.E(err) {
			t.Fatalf("error nsec back to secret key: '%s'", err)
			return
		}
		reSecBytes = reSec.Serialize()
		if string(secBytes) != string(reSecBytes) {
			t.Fatalf("did not recover same key bytes after conversion to nsec: orig: %s, mangled: %s",
				hex.EncodeToString(secBytes), hex.EncodeToString(reSecBytes))
		}
		if reNsec, err = SecretKeyToNsec(reSec); Chk.E(err) {
			t.Fatalf("error recovered secret key from converted to nsec: %s",
				err)
		}
		if !Equals(reNsec, nsec) {
			t.Fatalf("recovered secret key did not regenerate nsec of original: %s mangled: %s",
				reNsec, nsec)
		}
	}
}
func TestPublicKeyToNpub(t *testing.T) {
	var err error
	var sec *secp256k1.SecretKey
	var pub, rePub *secp256k1.PublicKey
	var npub, reNpub B
	var pubBytes, rePubBytes []byte
	for i := 0; i < 10000; i++ {
		if sec, err = secp256k1.GenerateSecretKey(); Chk.E(err) {
			t.Fatalf("error generating key: '%s'", err)
			return
		}
		pub = sec.PubKey()
		pubBytes = schnorr.SerializePubKey(pub)
		if npub, err = PublicKeyToNpub(pub); Chk.E(err) {
			t.Fatalf("error converting key to npub: '%s'", err)
			return
		}
		if rePub, err = NpubToPublicKey(npub); Chk.E(err) {
			t.Fatalf("error npub back to public key: '%s'", err)
			return
		}
		rePubBytes = schnorr.SerializePubKey(rePub)
		if string(pubBytes) != string(rePubBytes) {
			t.Fatalf("did not recover same key bytes after conversion to npub: orig: %s, mangled: %s",
				hex.EncodeToString(pubBytes), hex.EncodeToString(rePubBytes))
		}
		if reNpub, err = PublicKeyToNpub(rePub); Chk.E(err) {
			t.Fatalf("error recovered secret key from converted to nsec: %s", err)
		}
		if !Equals(reNpub, npub) {
			t.Fatalf("recovered public key did not regenerate npub of original: %s mangled: %s", reNpub, npub)
		}
	}
}
