package eventenvelope

import (
	"io"
	. "nostr.mleku.dev"

	"nostr.mleku.dev/codec/envelopes"
	"nostr.mleku.dev/codec/envelopes/enveloper"
	"nostr.mleku.dev/codec/event"
	sid "nostr.mleku.dev/codec/subscriptionid"
)

const L = "EVENT"

// Submission is a request from a client for a relay to store an event.
type Submission struct {
	*event.T
}

var _ enveloper.I = (*Submission)(nil)

func NewSubmission() *Submission                { return &Submission{T: &event.T{}} }
func NewSubmissionWith(ev *event.T) *Submission { return &Submission{T: ev} }
func (en *Submission) Label() string            { return L }

func (en *Submission) Write(w io.Writer) (err E) {
	var b B
	if b, err = en.MarshalJSON(b); Chk.E(err) {
		return
	}
	_, err = w.Write(b)
	return
}

func (en *Submission) MarshalJSON(dst B) (b B, err error) {
	b = dst
	b, err = envelopes.Marshal(b, L,
		func(bst B) (o B, err error) {
			o = bst
			if o, err = en.T.MarshalJSON(o); Chk.E(err) {
				return
			}
			return
		})
	return
}

func (en *Submission) UnmarshalJSON(b B) (r B, err error) {
	r = b
	en.T = event.New()
	if r, err = en.T.UnmarshalJSON(r); Chk.E(err) {
		return
	}
	if r, err = en.T.MarshalJSON(nil); Chk.E(err) {
		return
	}
	if r, err = envelopes.SkipToTheEnd(r); Chk.E(err) {
		return
	}
	return
}

func ParseSubmission(b B) (t *Submission, rem B, err E) {
	t = NewSubmission()
	if rem, err = t.UnmarshalJSON(b); Chk.E(err) {
		return
	}
	return
}

// Result is an event matching a filter associated with a subscription.
type Result struct {
	Subscription *sid.T
	Event        *event.T
}

var _ enveloper.I = (*Result)(nil)

func NewResult() *Result                          { return &Result{} }
func NewResultWith(s *sid.T, ev *event.T) *Result { return &Result{Subscription: s, Event: ev} }
func (en *Result) Label() S                       { return L }

func (en *Result) Write(w io.Writer) (err E) {
	var b B
	if b, err = en.MarshalJSON(b); Chk.E(err) {
		return
	}
	_, err = w.Write(b)
	return
}

func (en *Result) MarshalJSON(dst B) (b B, err error) {
	b = dst
	b, err = envelopes.Marshal(b, L,
		func(bst B) (o B, err error) {
			o = bst
			if o, err = en.Subscription.MarshalJSON(o); Chk.E(err) {
				return
			}
			o = append(o, ',')
			if o, err = en.Event.MarshalJSON(o); Chk.E(err) {
				return
			}
			return
		})
	return
}

func (en *Result) UnmarshalJSON(b B) (r B, err error) {
	r = b
	if en.Subscription, err = sid.New(B{0}); Chk.E(err) {
		return
	}
	if r, err = en.Subscription.UnmarshalJSON(r); Chk.E(err) {
		return
	}
	en.Event = event.New()
	if r, err = en.Event.UnmarshalJSON(r); Chk.E(err) {
		return
	}
	if r, err = envelopes.SkipToTheEnd(r); Chk.E(err) {
		return
	}
	return
}

func ParseResult(b B) (t *Result, rem B, err E) {
	t = NewResult()
	if rem, err = t.UnmarshalJSON(b); Chk.E(err) {
		return
	}
	return
}
