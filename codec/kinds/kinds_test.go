package kinds

import (
	. "nostr.mleku.dev"
	"testing"

	"lukechampine.com/frand"
	"nostr.mleku.dev/codec/kind"
)

func TestUnmarshalKindsArray(t *testing.T) {
	k := &T{make([]*kind.T, 100)}
	for i := range k.K {
		k.K[i] = kind.New(uint16(frand.Intn(65535)))
	}
	var dst B
	var err error
	if dst, err = k.MarshalJSON(dst); Chk.E(err) {
		t.Fatal(err)
	}
	k2 := &T{}
	var rem B
	if rem, err = k2.UnmarshalJSON(dst); Chk.E(err) {
		return
	}
	if len(rem) > 0 {
		t.Fatalf("failed to unmarshal, remnant afterwards '%s'", rem)
	}
	for i := range k.K {
		if *k.K[i] != *k2.K[i] {
			t.Fatalf("failed to unmarshal at element %d; got %x, expected %x",
				i, k.K[i], k2.K[i])
		}
	}
}
