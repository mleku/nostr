package closedenvelope

import (
	"io"
	. "nostr.mleku.dev"

	"nostr.mleku.dev/codec/envelopes"
	"nostr.mleku.dev/codec/envelopes/enveloper"
	"nostr.mleku.dev/codec/subscriptionid"
	"nostr.mleku.dev/codec/text"
)

const L = "CLOSED"

type T struct {
	Subscription *subscriptionid.T
	Reason       B
}

var _ enveloper.I = (*T)(nil)

func New() *T                                { return &T{Subscription: subscriptionid.NewStd()} }
func NewFrom(id *subscriptionid.T, msg B) *T { return &T{Subscription: id, Reason: msg} }
func (en *T) Label() string                  { return L }
func (en *T) ReasonString() string           { return S(en.Reason) }

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
			if o, err = en.Subscription.MarshalJSON(o); Chk.E(err) {
				return
			}
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
	if en.Subscription, err = subscriptionid.New(B{0}); Chk.E(err) {
		return
	}
	if r, err = en.Subscription.UnmarshalJSON(r); Chk.E(err) {
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
