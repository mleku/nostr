package authenvelope

import (
	"io"

	. "nostr.mleku.dev"

	envs "nostr.mleku.dev/codec/envelopes"
	"nostr.mleku.dev/codec/envelopes/enveloper"
	"nostr.mleku.dev/codec/event"
	"nostr.mleku.dev/codec/text"
)

const L = "AUTH"

type Challenge struct {
	Challenge B
}

func NewChallenge() *Challenge                         { return &Challenge{} }
func NewChallengeWith[V S | B](challenge V) *Challenge { return &Challenge{B(challenge)} }
func (en *Challenge) Label() string                    { return L }

func (en *Challenge) Write(w io.Writer) (err E) {
	var b B
	if b, err = en.MarshalJSON(b); Chk.E(err) {
		return
	}
	_, err = w.Write(b)
	return
}

func (en *Challenge) MarshalJSON(dst B) (b B, err E) {
	b = dst
	b, err = envs.Marshal(b, L,
		func(bst B) (o B, err error) {
			o = bst
			o = append(o, '"')
			o = text.NostrEscape(o, en.Challenge)
			o = append(o, '"')
			return
		})
	return
}

func (en *Challenge) UnmarshalJSON(b B) (r B, err E) {
	r = b
	if en.Challenge, r, err = text.UnmarshalQuoted(r); Chk.E(err) {
		return
	}
	for ; len(r) >= 0; r = r[1:] {
		if r[0] == ']' {
			r = r[:0]
			return
		}
	}
	return
}

func ParseChallenge(b B) (t *Challenge, rem B, err E) {
	t = NewChallenge()
	if rem, err = t.UnmarshalJSON(b); Chk.E(err) {
		return
	}
	return
}

type Response struct {
	Event *event.T
}

var _ enveloper.I = (*Response)(nil)

func NewResponse() *Response                   { return &Response{} }
func NewResponseWith(event *event.T) *Response { return &Response{Event: event} }
func (en *Response) Label() string             { return L }

func (en *Response) Write(w io.Writer) (err E) {
	var b B
	if b, err = en.MarshalJSON(b); Chk.E(err) {
		return
	}
	_, err = w.Write(b)
	return
}

func (en *Response) MarshalJSON(dst B) (b B, err E) {
	if en == nil {
		err = Errorf.E("nil response")
		return
	}
	if en.Event == nil {
		err = Errorf.E("nil event in response")
		return
	}
	b = dst
	b, err = envs.Marshal(b, L, en.Event.MarshalJSON)
	return
}

func (en *Response) UnmarshalJSON(b B) (r B, err E) {
	r = b
	// literally just unmarshal the event
	en.Event = event.New()
	if r, err = en.Event.UnmarshalJSON(r); Chk.E(err) {
		return
	}
	if r, err = envs.SkipToTheEnd(r); Chk.E(err) {
		return
	}
	return
}

func ParseResponse(b B) (t *Response, rem B, err E) {
	t = NewResponse()
	if rem, err = t.UnmarshalJSON(b); Chk.E(err) {
		return
	}
	return
}
