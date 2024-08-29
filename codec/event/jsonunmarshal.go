package event

import (
	"io"
	. "nostr.mleku.dev"

	"ec.mleku.dev/v2/schnorr"
	"github.com/minio/sha256-simd"
	"nostr.mleku.dev/codec/kind"
	"nostr.mleku.dev/codec/tags"
	"nostr.mleku.dev/codec/text"
	"nostr.mleku.dev/codec/timestamp"
)

func (ev *T) UnmarshalJSON(b B) (r B, err error) {
	key := make(B, 0, 9)
	r = b
	for ; len(r) > 0; r = r[1:] {
		if r[0] == '{' {
			r = r[1:]
			goto BetweenKeys
		}
	}
	goto eof
BetweenKeys:
	for ; len(r) > 0; r = r[1:] {
		if r[0] == '"' {
			r = r[1:]
			goto InKey
		}
	}
	goto eof
InKey:
	for ; len(r) > 0; r = r[1:] {
		if r[0] == '"' {
			r = r[1:]
			goto InKV
		}
		key = append(key, r[0])
	}
	goto eof
InKV:
	for ; len(r) > 0; r = r[1:] {
		if r[0] == ':' {
			r = r[1:]
			goto InVal
		}
	}
	goto eof
InVal:
	switch key[0] {
	case jId[0]:
		if !Equals(jId, key) {
			goto invalid
		}
		var id B
		if id, r, err = text.UnmarshalHex(r); Chk.E(err) {
			return
		}
		if len(id) != sha256.Size {
			err = Errorf.E("invalid ID, require %d got %d", sha256.Size,
				len(id))
			return
		}
		ev.ID = id
		goto BetweenKV
	case jPubkey[0]:
		if !Equals(jPubkey, key) {
			goto invalid
		}
		var pk B
		if pk, r, err = text.UnmarshalHex(r); Chk.E(err) {
			return
		}
		if len(pk) != schnorr.PubKeyBytesLen {
			err = Errorf.E("invalid pubkey, require %d got %d",
				schnorr.PubKeyBytesLen, len(pk))
			return
		}
		ev.PubKey = pk
		goto BetweenKV
	case jKind[0]:
		if !Equals(jKind, key) {
			goto invalid
		}
		ev.Kind = kind.New(0)
		if r, err = ev.Kind.UnmarshalJSON(r); Chk.E(err) {
			return
		}
		goto BetweenKV
	case jTags[0]:
		if !Equals(jTags, key) {
			goto invalid
		}
		ev.Tags = tags.New()
		if r, err = ev.Tags.UnmarshalJSON(r); Chk.E(err) {
			return
		}
		goto BetweenKV
	case jSig[0]:
		if !Equals(jSig, key) {
			goto invalid
		}
		var sig B
		if sig, r, err = text.UnmarshalHex(r); Chk.E(err) {
			return
		}
		if len(sig) != schnorr.SignatureSize {
			err = Errorf.E("invalid sig length, require %d got %d '%s'",
				schnorr.SignatureSize, len(sig), r)
			return
		}
		ev.Sig = sig
		goto BetweenKV
	case jContent[0]:
		if key[1] == jContent[1] {
			if !Equals(jContent, key) {
				goto invalid
			}
			if ev.Content, r, err = text.UnmarshalQuoted(r); Chk.E(err) {
				return
			}
			goto BetweenKV
		} else if key[1] == jCreatedAt[1] {
			if !Equals(jCreatedAt, key) {
				goto invalid
			}
			ev.CreatedAt = timestamp.New()
			if r, err = ev.CreatedAt.UnmarshalJSON(r); Chk.E(err) {
				return
			}
			goto BetweenKV
		} else {
			goto invalid
		}
	default:
		goto invalid
	}
BetweenKV:
	key = key[:0]
	for ; len(r) > 0; r = r[1:] {
		switch {
		case len(r) == 0:
			return
		case r[0] == '}':
			r = r[1:]
			goto AfterClose
		case r[0] == ',':
			r = r[1:]
			goto BetweenKeys
		case r[0] == '"':
			r = r[1:]
			goto InKey
		}
	}
	goto eof
AfterClose:
	return
invalid:
	err = Errorf.E("invalid key,\n'%s'\n'%s'\n'%s'", S(b), S(b[:len(r)]),
		S(r))
	return
eof:
	err = io.EOF
	return
}
