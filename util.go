package nostr

import (
	"bytes"
	"util.mleku.dev/context"
	"util.mleku.dev/lol"
)

type (
	B   = []byte
	S   = string
	E   = error
	N   = int
	Ctx = context.T
)

var (
	Log, Chk, Errorf = lol.Main.Log, lol.Main.Check, lol.Main.Errorf
	Equals           = bytes.Equal
	Compare          = bytes.Compare
)

func StringSliceToByteSlice(ss []S) (bs []B) {
	for _, s := range ss {
		bs = append(bs, B(s))
	}
	return
}
