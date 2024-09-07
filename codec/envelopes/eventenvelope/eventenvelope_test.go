package eventenvelope

import (
	"bufio"
	"bytes"
	"testing"

	. "nostr.mleku.dev"

	"nostr.mleku.dev/codec/envelopes"
	"nostr.mleku.dev/codec/event"
	"nostr.mleku.dev/codec/event/examples"
	"nostr.mleku.dev/codec/subscriptionid"
)

func TestSubmission(t *testing.T) {
	scanner := bufio.NewScanner(bytes.NewBuffer(examples.Cache))
	var c, rem, out B
	var err error
	for scanner.Scan() {
		b := scanner.Bytes()
		ev := event.New()
		if _, err = ev.UnmarshalJSON(b); Chk.E(err) {
			t.Fatal(err)
		}
		if len(rem) != 0 {
			t.Fatalf("some of input remaining after marshal/unmarshal: '%s'",
				rem)
		}
		rem = rem[:0]
		ea := NewSubmissionWith(ev)
		if rem, err = ea.MarshalJSON(rem); Chk.E(err) {
			t.Fatal(err)
		}
		c = append(c, rem...)
		var l string
		if l, rem, err = envelopes.Identify(rem); Chk.E(err) {
			t.Fatal(err)
		}
		if l != L {
			t.Fatalf("invalid sentinel %s, expect %s", l, L)
		}
		if rem, err = ea.UnmarshalJSON(rem); Chk.E(err) {
			t.Fatal(err)
		}
		if len(rem) != 0 {
			t.Fatalf("some of input remaining after marshal/unmarshal: '%s'",
				rem)
		}
		if out, err = ea.MarshalJSON(out); Chk.E(err) {
			t.Fatal(err)
		}
		if !Equals(out, c) {
			t.Fatalf("mismatched output\n%s\n\n%s\n", c, out)
		}
		c, out = c[:0], out[:0]
	}
}

func TestResult(t *testing.T) {
	scanner := bufio.NewScanner(bytes.NewBuffer(examples.Cache))
	var c, rem, out B
	var err error
	for scanner.Scan() {
		b := scanner.Bytes()
		ev := event.New()
		if _, err = ev.UnmarshalJSON(b); Chk.E(err) {
			t.Fatal(err)
		}
		if len(rem) != 0 {
			t.Fatalf("some of input remaining after marshal/unmarshal: '%s'",
				rem)
		}
		ea := NewResultWith(subscriptionid.NewStd().String(), ev)
		if rem, err = ea.MarshalJSON(rem); Chk.E(err) {
			t.Fatal(err)
		}
		c = append(c, rem...)
		var l string
		if l, rem, err = envelopes.Identify(rem); Chk.E(err) {
			t.Fatal(err)
		}
		if l != L {
			t.Fatalf("invalid sentinel %s, expect %s", l, L)
		}
		if rem, err = ea.UnmarshalJSON(rem); Chk.E(err) {
			t.Fatal(err)
		}
		if len(rem) != 0 {
			t.Fatalf("some of input remaining after marshal/unmarshal: '%s'",
				rem)
		}
		if out, err = ea.MarshalJSON(out); Chk.E(err) {
			t.Fatal(err)
		}
		if !Equals(out, c) {
			t.Fatalf("mismatched output\n%s\n\n%s\n", c, out)
		}
		rem, c, out = rem[:0], c[:0], out[:0]
	}
}
