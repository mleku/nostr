package auth

import (
	. "nostr.mleku.dev"
	"testing"

	"nostr.mleku.dev/crypto/p256k"
)

func TestCreateUnsigned(t *testing.T) {
	var err error
	signer := new(p256k.Signer)
	if err = signer.Generate(); Chk.E(err) {
		t.Fatal(err)
	}
	var ok bool
	const relayURL = "wss://example.com"
	for _ = range 100 {
		challenge := GenerateChallenge()
		ev := CreateUnsigned(signer.Pub(), challenge, relayURL)
		if err = ev.Sign(signer); Chk.E(err) {
			t.Fatal(err)
		}
		if ok, err = Validate(ev, challenge, relayURL); Chk.E(err) {
			t.Fatal(err)
		}
		if !ok {
			bb, _ := ev.MarshalJSON(nil)
			t.Fatalf("failed to validate auth event\n%s", bb)
		}
	}
}
