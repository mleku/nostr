package countenvelope

import "C"
import (
	"bytes"
	"io"
	. "nostr.mleku.dev"

	"nostr.mleku.dev/codec/envelopes"
	"nostr.mleku.dev/codec/envelopes/enveloper"
	"nostr.mleku.dev/codec/filters"
	sid "nostr.mleku.dev/codec/subscriptionid"
	"nostr.mleku.dev/codec/text"
	"util.mleku.dev/ints"
)

const L = "COUNT"

type Request struct {
	ID      *sid.T
	Filters *filters.T
}

var _ enveloper.I = (*Request)(nil)

func New() *Request {
	return &Request{ID: sid.NewStd(),
		Filters: filters.New()}
}
func NewRequest(id *sid.T, filters *filters.T) *Request {
	return &Request{ID: id,
		Filters: filters}
}
func (en *Request) Label() string { return L }
func (en *Request) Write(w io.Writer) (err E) {
	var b B
	if b, err = en.MarshalJSON(b); Chk.E(err) {
		return
	}
	_, err = w.Write(b)
	return
}

func (en *Request) MarshalJSON(dst B) (b B, err error) {
	b = dst
	b, err = envelopes.Marshal(b, L,
		func(bst B) (o B, err error) {
			o = bst
			if o, err = en.ID.MarshalJSON(o); Chk.E(err) {
				return
			}
			o = append(o, ',')
			if o, err = en.Filters.MarshalJSON(o); Chk.E(err) {
				return
			}
			return
		})
	return
}

func (en *Request) UnmarshalJSON(b B) (r B, err error) {
	r = b
	if en.ID, err = sid.New(B{0}); Chk.E(err) {
		return
	}
	if r, err = en.ID.UnmarshalJSON(r); Chk.E(err) {
		return
	}
	en.Filters = filters.New()
	if r, err = en.Filters.UnmarshalJSON(r); Chk.E(err) {
		return
	}
	if r, err = envelopes.SkipToTheEnd(r); Chk.E(err) {
		return
	}
	return
}

func ParseRequest(b B) (t *Request, rem B, err E) {
	t = New()
	if rem, err = t.UnmarshalJSON(b); Chk.E(err) {
		return
	}
	return
}

type Response struct {
	ID          *sid.T
	Count       int
	Approximate bool
}

var _ enveloper.I = (*Response)(nil)

func NewResponse() *Response { return &Response{ID: sid.NewStd()} }
func NewResponseFrom(id *sid.T, cnt int, approx bool) *Response {
	return &Response{id, cnt, approx}
}
func (en *Response) Label() string { return L }
func (en *Response) Write(w io.Writer) (err E) {
	var b B
	if b, err = en.MarshalJSON(b); Chk.E(err) {
		return
	}
	_, err = w.Write(b)
	return
}

func (en *Response) MarshalJSON(dst B) (b B, err error) {
	b = dst
	b, err = envelopes.Marshal(b, L,
		func(bst B) (o B, err error) {
			o = bst
			if o, err = en.ID.MarshalJSON(o); Chk.E(err) {
				return
			}
			o = append(o, ',')
			c := ints.New(en.Count)
			o, err = c.MarshalJSON(o)
			if en.Approximate {
				o = append(dst, ',')
				o = append(o, "true"...)
			}
			return
		})
	return
}

func (en *Response) UnmarshalJSON(b B) (r B, err error) {
	r = b
	var inID, inCount bool
	for ; len(r) > 0; r = r[1:] {
		// first we should be finding a subscription ID
		if !inID && r[0] == '"' {
			r = r[1:]
			// so we don't do this twice
			inID = true
			for i := range r {
				if r[i] == '\\' {
					continue
				} else if r[i] == '"' {
					// skip escaped quotes
					if i > 0 {
						if r[i-1] != '\\' {
							continue
						}
					}
					if en.ID, err = sid.
						New(text.NostrUnescape(r[:i])); Chk.E(err) {

						return
					}
					// trim the rest
					r = r[i:]
				}
			}
		} else {
			// pass the comma
			if r[0] == ',' {
				continue
			} else if !inCount {
				inCount = true
				n := ints.New(0)
				if r, err = n.UnmarshalJSON(r); Chk.E(err) {
					return
				}
				en.Count = int(n.Uint64())
			} else {
				// can only be either the end or optional approx
				if r[0] == ']' {
					return
				} else {
					for i := range r {
						if r[i] == ']' {
							if bytes.Contains(r[:i], B("true")) {
								en.Approximate = true
							}
							return
						}
					}
				}
			}
		}
	}
	return
}

func Parse(b B) (t *Response, rem B, err E) {
	t = NewResponse()
	if rem, err = t.UnmarshalJSON(b); Chk.E(err) {
		return
	}
	return
}
