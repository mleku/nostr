package btcec_test

import (
	"bytes"
	"os"

	"nostr.mleku.dev/util/lol"
)

type (
	B = []byte
	S = string
)

var (
	log, chk, errorf = lol.New(os.Stderr)
	equals           = bytes.Equal
)
