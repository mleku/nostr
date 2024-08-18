package enveloper

import (
	"nostr.mleku.dev/codec"
	"nostr.mleku.dev/protocol/relayws"
)

type I interface {
	Label() string
	Write(ws *relayws.WS) (err E)
	codec.JSON
}
