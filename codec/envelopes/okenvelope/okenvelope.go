package okenvelope

import (
	"io"

	. "nostr.mleku.dev"

	"nostr.mleku.dev/codec/envelopes"
	"nostr.mleku.dev/codec/envelopes/enveloper"
	"nostr.mleku.dev/codec/eventid"
	"nostr.mleku.dev/codec/text"
)

const (
	L = "OK"
)

type T struct {
	EventID *eventid.T
	OK      bool
	Reason  B
}

var _ enveloper.I = (*T)(nil)

func New() *T { return &T{} }
func NewFrom[V S | B](eid V, ok bool, msg ...B) *T {
	var m B
	if len(msg) > 0 {
		m = msg[0]
	}
	return &T{EventID: eventid.NewWith(eid), OK: ok, Reason: m}
}
func (en *T) Label() string        { return L }
func (en *T) ReasonString() string { return S(en.Reason) }

func (en *T) Write(w io.Writer) (err E) {
	var b B
	if b, err = en.MarshalJSON(b); Chk.E(err) {
		return
	}
	_, err = w.Write(b)
	return
}

func (en *T) MarshalJSON(dst B) (b B, err error) {
	b = dst
	b, err = envelopes.Marshal(b, L,
		func(bst B) (o B, err error) {
			o = bst
			o = append(o, '"')
			o = en.EventID.ByteString(o)
			o = append(o, '"')
			o = append(o, ',')
			o = text.MarshalBool(o, en.OK)
			o = append(o, ',')
			o = append(o, '"')
			o = text.NostrEscape(o, en.Reason)
			o = append(o, '"')
			return
		})
	return
}

func (en *T) UnmarshalJSON(b B) (r B, err error) {
	r = b
	var idHex B
	if idHex, r, err = text.UnmarshalHex(r); Chk.E(err) {
		return
	}
	if en.EventID, err = eventid.NewFromBytes(idHex); Chk.E(err) {
		return
	}
	if r, err = text.Comma(r); Chk.E(err) {
		return
	}
	if r, en.OK, err = text.UnmarshalBool(r); Chk.E(err) {
		return
	}
	if r, err = text.Comma(r); Chk.E(err) {
		return
	}
	if en.Reason, r, err = text.UnmarshalQuoted(r); Chk.E(err) {
		return
	}
	if r, err = envelopes.SkipToTheEnd(r); Chk.E(err) {
		return
	}
	return
}

func Parse(b B) (t *T, rem B, err E) {
	t = New()
	if rem, err = t.UnmarshalJSON(b); Chk.E(err) {
		return
	}
	return
}
