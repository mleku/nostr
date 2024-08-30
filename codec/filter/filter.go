package filter

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"

	"ec.mleku.dev/v2/schnorr"
	"ec.mleku.dev/v2/secp256k1"
	"github.com/minio/sha256-simd"
	"lukechampine.com/frand"
	. "nostr.mleku.dev"
	"nostr.mleku.dev/codec/event"
	"nostr.mleku.dev/codec/kind"
	"nostr.mleku.dev/codec/kinds"
	"nostr.mleku.dev/codec/tag"
	"nostr.mleku.dev/codec/tags"
	"nostr.mleku.dev/codec/text"
	"nostr.mleku.dev/codec/timestamp"
	"util.mleku.dev/hex"
	"util.mleku.dev/ints"
)

// T is the primary query form for requesting events from a nostr relay.
//
// The ordering of fields of filters is not specified as in the protocol there is no requirement
// to generate a hash for fast recognition of identical filters. However, for internal use in a
// relay, by applying a consistent sort order, this library will produce an identical JSON from
// the same *set* of fields no matter what order they were provided.
//
// This is to facilitate the deduplication of filters so an effective identical match is not
// performed on an identical filter.
type T struct {
	IDs     *tag.T       `json:"ids,omitempty"`
	Kinds   *kinds.T     `json:"kinds,omitempty"`
	Authors *tag.T       `json:"authors,omitempty"`
	Tags    *tags.T      `json:"-,omitempty"`
	Since   *timestamp.T `json:"since,omitempty"`
	Until   *timestamp.T `json:"until,omitempty"`
	Search  B            `json:"search,omitempty"`
	Limit   int          `json:"limit,omitempty"`
}

func New() (f *T) {
	return &T{
		IDs:     tag.NewWithCap(100),
		Kinds:   kinds.NewWithCap(10),
		Authors: tag.NewWithCap(100),
		Tags:    tags.New(),
		Since:   timestamp.New(),
		Until:   timestamp.New(),
		Search:  nil,
		Limit:   0,
	}
}

// Clone creates a new filter with all the same elements in them, because they are immutable,
// basically, except setting the Limit field as 1, because it is used in the subscription
// management code to act as a reference counter, and making a clone implicitly means 1
// reference.
func (f *T) Clone() (clone *T) {
	return &T{
		IDs:     f.IDs,
		Kinds:   f.Kinds,
		Authors: f.Authors,
		Tags:    f.Tags,
		Since:   f.Since,
		Until:   f.Until,
		Search:  f.Search,
		Limit:   1,
	}
}

var (
	IDs     = B("ids")
	Kinds   = B("kinds")
	Authors = B("authors")
	Tags    = B("tags")
	Since   = B("since")
	Until   = B("until")
	Limit   = B("limit")
	Search  = B("search")
)

func (f *T) MarshalJSON(dst B) (b B, err error) {
	var first bool
	// sort the fields so they come out the same
	f.Sort()
	// open parentheses
	dst = append(dst, '{')
	if f.IDs != nil && len(f.IDs.Field) > 0 {
		first = true
		dst = text.JSONKey(dst, IDs)
		dst = text.MarshalHexArray(dst, f.IDs.ToByteSlice())
	}
	if f.Kinds != nil && len(f.Kinds.K) > 0 {
		if first {
			dst = append(dst, ',')
		} else {
			first = true
		}
		dst = text.JSONKey(dst, Kinds)
		if dst, err = f.Kinds.MarshalJSON(dst); Chk.E(err) {
			return
		}
	}
	if f.Authors != nil && len(f.Authors.Field) > 0 {
		if first {
			dst = append(dst, ',')
		} else {
			first = true
		}
		dst = text.JSONKey(dst, Authors)
		dst = text.MarshalHexArray(dst, f.Authors.ToByteSlice())
	}
	if f.Tags != nil && len(f.Tags.T) > 0 {
		if first {
			dst = append(dst, ',')
		} else {
			first = true
		}
		dst = text.JSONKey(dst, Tags)
		if dst, err = f.Tags.MarshalJSON(dst); Chk.E(err) {
			return
		}
	}
	if f.Since != nil && f.Since.U64() > 0 {
		if first {
			dst = append(dst, ',')
		} else {
			first = true
		}
		dst = text.JSONKey(dst, Since)
		if dst, err = f.Since.MarshalJSON(dst); Chk.E(err) {
			return
		}
	}
	if f.Until != nil && f.Until.U64() > 0 {
		if first {
			dst = append(dst, ',')
		} else {
			first = true
		}
		dst = text.JSONKey(dst, Until)
		if dst, err = f.Until.MarshalJSON(dst); Chk.E(err) {
			return
		}
	}
	if len(f.Search) > 0 {
		if first {
			dst = append(dst, ',')
		} else {
			first = true
		}
		dst = text.JSONKey(dst, Search)
		dst = text.AppendQuote(dst, f.Search, text.NostrEscape)
	}
	if f.Limit > 0 {
		if first {
			dst = append(dst, ',')
		} else {
			first = true
		}
		dst = text.JSONKey(dst, Limit)
		if dst, err = ints.New(f.Limit).MarshalJSON(dst); Chk.E(err) {
			return
		}
	}
	// close parentheses
	dst = append(dst, '}')
	b = dst
	return
}

func (f *T) Serialize() (b B) {
	b, _ = f.MarshalJSON(nil)
	return
}

func (f *T) String() (r S) { return S(f.Serialize()) }

// states of the unmarshaler
const (
	beforeOpen = iota
	openParen
	inKey
	inKV
	inVal
	betweenKV
	afterClose
)

func (f *T) UnmarshalJSON(b B) (r B, err error) {
	r = b[:]
	var key B
	var state int
	for ; len(r) >= 0; r = r[1:] {
		// Log.I.F("%c", rem[0])
		switch state {
		case beforeOpen:
			if r[0] == '{' {
				state = openParen
				// Log.I.Ln("openParen")
			}
		case openParen:
			if r[0] == '"' {
				state = inKey
				// Log.I.Ln("inKey")
			}
		case inKey:
			if r[0] == '"' {
				state = inKV
				// Log.I.Ln("inKV")
			} else {
				key = append(key, r[0])
			}
		case inKV:
			if r[0] == ':' {
				state = inVal
			}
		case inVal:
			if len(key) < 1 {
				err = Errorf.E("filter key zero length: '%s'\n'%s", b, r)
				return
			}
			switch key[0] {
			case '#':
				r = r[1:]
				switch r[0] {
				case 'e', 'p':
					// the tags must all be 64 character hexadecimal
					var ff []B
					if ff, r, err = text.UnmarshalHexArray(r,
						sha256.Size); Chk.E(err) {
						return
					}
					ff = append([]B{key}, ff...)
					f.Tags.T = append(f.Tags.T, tag.New(ff...))
				default:
					// other types of tags can be anything
					var ff []B
					if ff, r, err = text.UnmarshalStringArray(r); Chk.E(err) {
						return
					}
					ff = append([]B{key}, ff...)
					f.Tags.T = append(f.Tags.T, tag.New(ff...))
				}
				state = betweenKV
			case IDs[0]:
				if len(key) < len(IDs) {
					goto invalid
				}
				var ff []B
				if ff, r, err = text.UnmarshalHexArray(r,
					sha256.Size); Chk.E(err) {
					return
				}
				f.IDs = tag.New(ff...)
				state = betweenKV
			case Kinds[0]:
				if len(key) < len(Kinds) {
					goto invalid
				}
				f.Kinds = kinds.NewWithCap(0)
				if r, err = f.Kinds.UnmarshalJSON(r); Chk.E(err) {
					return
				}
				state = betweenKV
			case Authors[0]:
				if len(key) < len(Authors) {
					goto invalid
				}
				var ff []B
				if ff, r, err = text.UnmarshalHexArray(r, schnorr.PubKeyBytesLen); Chk.E(err) {
					return
				}
				f.Authors = tag.New(ff...)
				state = betweenKV
			case Until[0]:
				if len(key) < len(Until) {
					goto invalid
				}
				u := ints.New(0)
				if r, err = u.UnmarshalJSON(r); Chk.E(err) {
					return
				}
				f.Until = timestamp.FromUnix(int64(u.N))
				state = betweenKV
			case Limit[0]:
				if len(key) < len(Limit) {
					goto invalid
				}
				l := ints.New(0)
				if r, err = l.UnmarshalJSON(r); Chk.E(err) {
					return
				}
				f.Limit = int(l.N)
				state = betweenKV
			case Search[0]:
				if len(key) < len(Since) {
					goto invalid
				}
				switch key[1] {
				case Search[1]:
					if len(key) < len(Search) {
						goto invalid
					}
					var txt B
					if txt, r, err = text.UnmarshalQuoted(r); Chk.E(err) {
						return
					}
					f.Search = txt
					// Log.I.F("\n%s\n%s", txt, rem)
					state = betweenKV
					// Log.I.Ln("betweenKV")
				case Since[1]:
					if len(key) < len(Since) {
						goto invalid
					}
					s := ints.New(0)
					if r, err = s.UnmarshalJSON(r); Chk.E(err) {
						return
					}
					f.Since = timestamp.FromUnix(int64(s.N))
					state = betweenKV
					// Log.I.Ln("betweenKV")
				}
			default:
				goto invalid
			}
			key = key[:0]
		case betweenKV:
			if len(r) == 0 {
				return
			}
			if r[0] == '}' {
				state = afterClose
				// Log.I.Ln("afterClose")
				// rem = rem[1:]
			} else if r[0] == ',' {
				state = openParen
				// Log.I.Ln("openParen")
			} else if r[0] == '"' {
				state = inKey
				// Log.I.Ln("inKey")
			}
		}
		if len(r) == 0 {
			return
		}
		if r[0] == '}' {
			r = r[1:]
			return
		}
	}
invalid:
	err = Errorf.E("invalid key,\n'%s'\n'%s'", S(b), S(r))
	return
}

func (f *T) Matches(ev *event.T) bool {
	if ev == nil {
		// Log.T.F("nil event")
		return false
	}
	if f.IDs != nil && len(f.IDs.Field) > 0 && !f.IDs.Contains(ev.ID) {
		// Log.T.F("no ids in filter match event\nEVENT %s\nFILTER %s", ev.ToObject().String(), f.ToObject().String())
		return false
	}
	if f.Kinds != nil && len(f.Kinds.K) > 0 && !f.Kinds.Contains(ev.Kind) {
		// Log.T.F("no matching kinds in filter\nEVENT %s\nFILTER %s", ev.ToObject().String(), f.ToObject().String())
		return false
	}
	if f.Authors != nil && len(f.Authors.Field) > 0 && !f.Authors.Contains(ev.PubKey) {
		// Log.T.F("no matching authors in filter\nEVENT %s\nFILTER %s", ev.ToObject().String(), f.ToObject().String())
		return false
	}
	if f.Tags != nil {
		for i, v := range f.Tags.T {
			// remove the hash prefix (idk why this thing even exists tbh)
			if bytes.HasPrefix(v.Field[0], B("#")) {
				f.Tags.T[i].Field[0] = f.Tags.T[i].Field[0][1:]
			}
			if len(v.Field) > 0 && !ev.Tags.ContainsAny(v.Field[0], v.ToByteSlice()...) {
				// Log.T.F("no matching tags in filter\nEVENT %s\nFILTER %s", ev.ToObject().String(), f.ToObject().String())
				return false
			}
			// special case for p tags
		}
	}
	if f.Since != nil && f.Since.Int() != 0 && ev.CreatedAt != nil && ev.CreatedAt.I64() < f.Since.I64() {
		// Log.T.F("event is older than since\nEVENT %s\nFILTER %s", ev.ToObject().String(), f.ToObject().String())
		return false
	}
	if f.Until != nil && f.Until.Int() != 0 && ev.CreatedAt.I64() > f.Until.I64() {
		// Log.T.F("event is newer than until\nEVENT %s\nFILTER %s", ev.ToObject().String(), f.ToObject().String())
		return false
	}
	return true
}

// Fingerprint returns an 8 byte truncated sha256 hash of the filter in the canonical form
// created by MarshalJSON.
//
// This hash is generated via the JSON encoded form of the filter, with the Limit field removed.
// This value should be set to zero after all results from a query of stored events, as per
// NIP-01.
func (f *T) Fingerprint() (fp uint64, err E) {
	lim := f.Limit
	f.Limit = 0
	var b B
	if b, err = f.MarshalJSON(b); Chk.E(err) {
		return
	}
	h := sha256.Sum256(b)
	hb := h[:]
	fp = binary.LittleEndian.Uint64(hb)
	f.Limit = lim
	return
}

// Sort the fields of a filter so a fingerprint on a filter that has the same set of content
// produces the same fingerprint.
func (f *T) Sort() {
	sort.Sort(f.IDs)
	sort.Sort(f.Kinds)
	sort.Sort(f.Authors)
	sort.Sort(f.Tags)
}

func arePointerValuesEqual[V comparable](a *V, b *V) bool {
	if a == nil && b == nil {
		return true
	}
	if a != nil && b != nil {
		return *a == *b
	}
	return false
}

func Equal(a, b *T) bool {
	// switch is a convenient way to bundle a long list of tests like this:
	if !a.Kinds.Equals(b.Kinds) ||
		!a.IDs.Equal(b.IDs) ||
		!a.Authors.Equal(b.Authors) ||
		len(a.Tags.T) != len(b.Tags.T) ||
		!arePointerValuesEqual(a.Since, b.Since) ||
		!arePointerValuesEqual(a.Until, b.Until) ||
		!Equals(a.Search, b.Search) ||
		!a.Tags.Equal(b.Tags) {
		return false
	}
	return true
}

func GenFilter() (f *T, err error) {
	f = New()
	n := frand.Intn(16)
	for _ = range n {
		id := make(B, sha256.Size)
		frand.Read(id)
		f.IDs.Field = append(f.IDs.Field, id)
	}
	n = frand.Intn(16)
	for _ = range n {
		f.Kinds.K = append(f.Kinds.K, kind.New(frand.Intn(65535)))
	}
	n = frand.Intn(16)
	for _ = range n {
		var sk *secp256k1.SecretKey
		if sk, err = secp256k1.GenerateSecretKey(); Chk.E(err) {
			return
		}
		pk := sk.PubKey()
		f.Authors.Field = append(f.Authors.Field, schnorr.SerializePubKey(pk))
	}
	a := frand.Intn(16)
	if a < n {
		n = a
	}
	for i := range n {
		p := make(B, 0, schnorr.PubKeyBytesLen*2)
		p = hex.EncAppend(p, f.Authors.Field[i])
		f.Tags.T = append(f.Tags.T, tag.New(B("p"), p))
		idb := make(B, sha256.Size)
		frand.Read(idb)
		id := make(B, 0, sha256.Size*2)
		id = hex.EncAppend(id, idb)
		f.Tags.T = append(f.Tags.T, tag.New(B("e"), id))
		f.Tags.T = append(f.Tags.T,
			tag.New(B("a"),
				B(fmt.Sprintf("%d:%s:", frand.Intn(65535), id))))
	}
	tn := int(timestamp.Now().I64())
	before := timestamp.T(tn - frand.Intn(10000))
	f.Since = &before
	f.Until = timestamp.Now()
	f.Search = B("token search text")
	return
}
