package tests

import (
	"encoding/base64"
	. "nostr.mleku.dev"

	"lukechampine.com/frand"
	"nostr.mleku.dev/codec/event"
	"nostr.mleku.dev/codec/kind"
	"nostr.mleku.dev/codec/timestamp"
	"nostr.mleku.dev/crypto/p256k"
	"util.mleku.dev/hex"
)

func GenerateEvent(nsec B, maxSize int) (ev *event.T, binSize int, err E) {
	l := frand.Intn(maxSize * 6 / 8) // account for base64 expansion
	ev = &event.T{
		Kind:      kind.TextNote,
		CreatedAt: timestamp.Now(),
		Content:   B(base64.StdEncoding.EncodeToString(frand.Bytes(l))),
	}
	var sec B
	if _, err = hex.DecBytes(sec, nsec); Chk.E(err) {
		return
	}
	signer := new(p256k.Signer)
	if err = signer.Generate(); Chk.E(err) {
		return
	}
	if err = signer.InitSec(sec); Chk.E(err) {
		return
	}
	if err = ev.Sign(signer); Chk.E(err) {
		return
	}
	var bin []byte
	bin, err = ev.MarshalBinary(bin)
	binSize = len(bin)
	return
}
