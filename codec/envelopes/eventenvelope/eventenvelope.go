package eventenvelope

import (
	"nostr.mleku.dev/codec/envelopes"
	"nostr.mleku.dev/codec/envelopes/enveloper"
	"nostr.mleku.dev/codec/event"
	sid "nostr.mleku.dev/codec/subscriptionid"
	"nostr.mleku.dev/protocol/relayws"
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

func (en *Submission) Write(ws *relayws.WS) (err E) {
	var b B
	if b, err = en.MarshalJSON(b); chk.E(err) {
		return
	}
	return ws.WriteTextMessage(b)
}

func (en *Submission) MarshalJSON(dst B) (b B, err error) {
	b = dst
	b, err = envelopes.Marshal(b, L,
		func(bst B) (o B, err error) {
			o = bst
			if o, err = en.T.MarshalJSON(o); chk.E(err) {
				return
			}
			return
		})
	return
}

func (en *Submission) UnmarshalJSON(b B) (r B, err error) {
	r = b
	log.W.F("%s", r)
	en.T = event.New()
	if r, err = en.T.UnmarshalJSON(r); chk.E(err) {
		return
	}
	var bb B
	bb, err = en.T.MarshalJSON(nil)
	log.I.F("%s", bb)
	if r, err = envelopes.SkipToTheEnd(r); chk.E(err) {
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

func (en *Result) Write(ws *relayws.WS) (err E) {
	var b B
	if b, err = en.MarshalJSON(b); chk.E(err) {
		return
	}
	return ws.WriteTextMessage(b)
}

func (en *Result) MarshalJSON(dst B) (b B, err error) {
	b = dst
	b, err = envelopes.Marshal(b, L,
		func(bst B) (o B, err error) {
			o = bst
			if o, err = en.Subscription.MarshalJSON(o); chk.E(err) {
				return
			}
			o = append(o, ',')
			if o, err = en.Event.MarshalJSON(o); chk.E(err) {
				return
			}
			return
		})
	return
}

func (en *Result) UnmarshalJSON(b B) (r B, err error) {
	r = b
	if en.Subscription, err = sid.New(B{0}); chk.E(err) {
		return
	}
	if r, err = en.Subscription.UnmarshalJSON(r); chk.E(err) {
		return
	}
	en.Event = event.New()
	if r, err = en.Event.UnmarshalJSON(r); chk.E(err) {
		return
	}
	if r, err = envelopes.SkipToTheEnd(r); chk.E(err) {
		return
	}
	return
}
