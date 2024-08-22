package enveloper

import (
	"io"

	"nostr.mleku.dev/codec"
)

type I interface {
	Label() string
	Write(w io.Writer) (err E)
	codec.JSON
}
