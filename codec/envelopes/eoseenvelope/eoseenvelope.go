package eoseenvelope

import (
	"io"
	. "nostr.mleku.dev"

	"nostr.mleku.dev/codec/envelopes"
	"nostr.mleku.dev/codec/envelopes/enveloper"
	sid "nostr.mleku.dev/codec/subscriptionid"
)

const L = "EOSE"

type T struct {
	Subscription *sid.T
}

var _ enveloper.I = (*T)(nil)

func New() *T               { return &T{Subscription: sid.NewStd()} }
func NewFrom(id *sid.T) *T  { return &T{Subscription: id} }
func (en *T) Label() string { return L }

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
			return
		},
	)
	return
}

func (en *T) UnmarshalJSON(b B) (r B, err error) {
	r = b
	if en.Subscription, err = sid.New(B{0}); Chk.E(err) {
		return
	}
	if r, err = en.Subscription.UnmarshalJSON(r); Chk.E(err) {
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
