package btcec_test

import (
	"bytes"
	"os"

	"util.mleku.dev/lol"
)

type (
	B = []byte
	S = string
)

var (
	log, chk, errorf = lol.New(os.Stderr)
	equals           = bytes.Equal
)
