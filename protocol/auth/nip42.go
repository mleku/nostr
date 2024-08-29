package auth

import (
	"crypto/rand"
	"encoding/base64"
	"net/url"
	. "nostr.mleku.dev"
	"strings"
	"time"

	"nostr.mleku.dev/codec/event"
	"nostr.mleku.dev/codec/kind"
	"nostr.mleku.dev/codec/tag"
	"nostr.mleku.dev/codec/tags"
	"nostr.mleku.dev/codec/timestamp"
)

// GenerateChallenge creates a reasonable, 96 byte base64 challenge string
func GenerateChallenge() (b B) {
	bb := make(B, 12)
	b = make(B, 16)
	_, _ = rand.Read(bb)
	base64.StdEncoding.Encode(b, bb)
	return
}

// CreateUnsigned creates an event which should be sent via an "AUTH" command.
// If the authentication succeeds, the user will be authenticated as pubkey.
func CreateUnsigned(pubkey, challenge B, relayURL string) (ev *event.T) {
	return &event.T{
		PubKey:    pubkey,
		CreatedAt: timestamp.Now(),
		Kind:      kind.ClientAuthentication,
		Tags: tags.New(tag.New("relay", relayURL),
			tag.New("challenge", string(challenge))),
	}
}

// helper function for ValidateAuthEvent.
func parseURL(input string) (*url.URL, error) {
	return url.Parse(
		strings.ToLower(
			strings.TrimSuffix(input, "/"),
		),
	)
}

var ChallengeTag = B("challenge")
var RelayTag = B("relay")

// Validate checks whether event is a valid NIP-42 event for given challenge and relayURL.
// The result of the validation is encoded in the ok bool.
func Validate(evt *event.T, challenge B, relayURL S) (ok bool, err error) {
	if evt.Kind != kind.ClientAuthentication {
		err = Log.E.Err("event incorrect kind for auth: %d %s",
			evt.Kind, kind.Map[evt.Kind])
		Log.D.Ln(err)
		return
	}
	if evt.Tags.GetFirst(tag.New(ChallengeTag, challenge)) == nil {
		err = Log.E.Err("challenge tag missing from auth response")
		Log.D.Ln(err)
		return
	}
	// Log.I.Ln(relayURL)
	var expected, found *url.URL
	if expected, err = parseURL(relayURL); Chk.D(err) {
		Log.D.Ln(err)
		return
	}
	r := evt.Tags.
		GetFirst(tag.New(RelayTag, nil)).Value()
	if len(r) == 0 {
		err = Log.E.Err("relay tag missing from auth response")
		Log.D.Ln(err)
		return
	}
	if found, err = parseURL(string(r)); Chk.D(err) {
		err = Log.E.Err("error parsing relay url: %s", err)
		Log.D.Ln(err)
		return
	}
	if expected.Scheme != found.Scheme {
		err = Log.E.Err("HTTP Scheme incorrect: expected '%s' got '%s",
			expected.Scheme, found.Scheme)
		Log.D.Ln(err)
		return
	}
	if expected.Host != found.Host {
		err = Log.E.Err("HTTP Host incorrect: expected '%s' got '%s",
			expected.Host, found.Host)
		Log.D.Ln(err)
		return
	}
	if expected.Path != found.Path {
		err = Log.E.Err("HTTP Path incorrect: expected '%s' got '%s",
			expected.Path, found.Path)
		Log.D.Ln(err)
		return
	}

	now := time.Now()
	if evt.CreatedAt.Time().After(now.Add(10*time.Minute)) ||
		evt.CreatedAt.Time().Before(now.Add(-10*time.Minute)) {
		err = Log.E.Err(
			"auth event more than 10 minutes before or after current time")
		Log.D.Ln(err)
		return
	}
	// save for last, as it is most expensive operation
	return evt.Verify()
}
