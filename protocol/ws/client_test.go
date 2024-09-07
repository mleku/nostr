package ws

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/websocket"
	. "nostr.mleku.dev"
	"nostr.mleku.dev/codec/envelopes/eventenvelope"
	"nostr.mleku.dev/codec/envelopes/okenvelope"
	"nostr.mleku.dev/codec/event"
	"nostr.mleku.dev/codec/eventid"
	"nostr.mleku.dev/codec/kind"
	"nostr.mleku.dev/codec/tag"
	"nostr.mleku.dev/codec/tags"
	"nostr.mleku.dev/codec/timestamp"
	"nostr.mleku.dev/crypto/p256k"
	"util.mleku.dev/normalize"
)

func TestPublish(t *testing.T) {
	// test note to be sent over websocket
	var err E
	signer := &p256k.Signer{}
	if err = signer.Generate(); Chk.E(err) {
		t.Fatal(err)
	}
	textNote := &event.T{
		Kind:      kind.TextNote,
		Content:   B("hello"),
		CreatedAt: timestamp.FromUnix(1672068534), // random fixed timestamp
		Tags:      tags.New(tag.New("foo", "bar")),
		PubKey:    signer.Pub(),
	}
	if err = textNote.Sign(signer); Chk.E(err) {
		t.Fatalf("textNote.Sign: %v", err)
	}
	// fake relay server
	var mu sync.Mutex // guards published to satisfy go test -race
	var published bool
	ws := newWebsocketServer(func(conn *websocket.Conn) {
		mu.Lock()
		published = true
		mu.Unlock()
		// verify the client sent exactly the textNote
		var raw []json.RawMessage
		if err := websocket.JSON.Receive(conn, &raw); err != nil {
			t.Errorf("websocket.JSON.Receive: %v", err)
		}

		if S(raw[0]) != fmt.Sprintf(`"%s"`, eventenvelope.L) {
			t.Errorf("got type %s, want %s", raw[0], eventenvelope.L)
		}
		env := eventenvelope.NewSubmission()
		if raw[1], err = env.UnmarshalJSON(raw[1]); Chk.E(err) {
			t.Fatal(err)
		}
		// event := parseEventMessage(t, raw)
		if !bytes.Equal(env.T.Serialize(), textNote.Serialize()) {
			t.Errorf("received event:\n%s\nwant:\n%s", env.T.Serialize(), textNote.Serialize())
		}
		// send back an ok nip-20 command result
		var eid *eventid.T
		if eid, err = eventid.NewFromBytes(textNote.ID); Chk.E(err) {
			t.Fatal(err)
		}
		var res B
		if res, err = okenvelope.NewFrom(eid.Bytes(), true, nil).MarshalJSON(res); Chk.E(err) {
			t.Fatal(err)
		}
		if err := websocket.Message.Send(conn, res); err != nil {
			t.Errorf("websocket.JSON.Send: %v", err)
		}
	})
	defer ws.Close()
	// connect a client and send the text note
	rl := mustRelayConnect(ws.URL)
	err = rl.Publish(context.Background(), textNote)
	if err != nil {
		t.Errorf("publish should have succeeded")
	}
	if !published {
		t.Errorf("fake relay server saw no event")
	}
}

func TestPublishBlocked(t *testing.T) {
	// test note to be sent over websocket
	var err E
	signer := &p256k.Signer{}
	if err = signer.Generate(); Chk.E(err) {
		t.Fatal(err)
	}
	textNote := &event.T{
		Kind:      kind.TextNote,
		Content:   B("hello"),
		CreatedAt: timestamp.FromUnix(1672068534), // random fixed timestamp
		PubKey:    signer.Pub(),
	}
	if err = textNote.Sign(signer); Chk.E(err) {
		t.Fatalf("textNote.Sign: %v", err)
	}
	// fake relay server
	ws := newWebsocketServer(func(conn *websocket.Conn) {
		// discard received message; not interested
		var raw []json.RawMessage
		if err := websocket.JSON.Receive(conn, &raw); err != nil {
			t.Errorf("websocket.JSON.Receive: %v", err)
		}
		// send back a not ok nip-20 command result
		var eid *eventid.T
		if eid, err = eventid.NewFromBytes(textNote.ID); Chk.E(err) {
			t.Fatal(err)
		}
		var res B
		if res, err = okenvelope.NewFrom(eid.Bytes(), false,
			normalize.Msg(normalize.Blocked, "no reason")).MarshalJSON(res); Chk.E(err) {
			t.Fatal(err)
		}
		if err := websocket.Message.Send(conn, res); err != nil {
			t.Errorf("websocket.JSON.Send: %v", err)
		}
		// res := []any{"OK", textNote.ID, false, "blocked"}
		websocket.JSON.Send(conn, res)
	})
	defer ws.Close()

	// connect a client and send a text note
	rl := mustRelayConnect(ws.URL)
	if err = rl.Publish(context.Background(), textNote); !Chk.E(err) {
		t.Errorf("should have failed to publish")
	}
}

func TestPublishWriteFailed(t *testing.T) {
	// test note to be sent over websocket
	var err E
	signer := &p256k.Signer{}
	if err = signer.Generate(); Chk.E(err) {
		t.Fatal(err)
	}
	textNote := &event.T{
		Kind:      kind.TextNote,
		Content:   B("hello"),
		CreatedAt: timestamp.FromUnix(1672068534), // random fixed timestamp
		PubKey:    signer.Pub(),
	}
	if err = textNote.Sign(signer); Chk.E(err) {
		t.Fatalf("textNote.Sign: %v", err)
	}
	// fake relay server
	ws := newWebsocketServer(func(conn *websocket.Conn) {
		// reject receive - force send error
		conn.Close()
	})
	defer ws.Close()

	// connect a client and send a text note
	rl := mustRelayConnect(ws.URL)
	// Force brief period of time so that publish always fails on closed socket.
	time.Sleep(1 * time.Millisecond)
	err = rl.Publish(context.Background(), textNote)
	if err == nil {
		t.Errorf("should have failed to publish")
	}
}

func TestConnectContext(t *testing.T) {
	// fake relay server
	var mu sync.Mutex // guards connected to satisfy go test -race
	var connected bool
	ws := newWebsocketServer(func(conn *websocket.Conn) {
		mu.Lock()
		connected = true
		mu.Unlock()
		io.ReadAll(conn) // discard all input
	})
	defer ws.Close()

	// relay client
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	r, err := RelayConnect(ctx, ws.URL)
	if err != nil {
		t.Fatalf("RelayConnectContext: %v", err)
	}
	defer r.Close()

	mu.Lock()
	defer mu.Unlock()
	if !connected {
		t.Error("fake relay server saw no client connect")
	}
}

func TestConnectContextCanceled(t *testing.T) {
	// fake relay server
	ws := newWebsocketServer(discardingHandler)
	defer ws.Close()

	// relay client
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // make ctx expired
	_, err := RelayConnect(ctx, ws.URL)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("RelayConnectContext returned %v error; want context.Canceled", err)
	}
}

func TestConnectWithOrigin(t *testing.T) {
	// fake relay server
	// default handler requires origin golang.org/x/net/websocket
	ws := httptest.NewServer(websocket.Handler(discardingHandler))
	defer ws.Close()

	// relay client
	r := NewRelay(context.Background(), S(normalize.URL(ws.URL)))
	r.RequestHeader = http.Header{"origin": {"https://example.com"}}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := r.Connect(ctx)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func discardingHandler(conn *websocket.Conn) {
	io.ReadAll(conn) // discard all input
}

func newWebsocketServer(handler func(*websocket.Conn)) *httptest.Server {
	return httptest.NewServer(&websocket.Server{
		Handshake: anyOriginHandshake,
		Handler:   handler,
	})
}

// anyOriginHandshake is an alternative to default in golang.org/x/net/websocket
// which checks for origin. nostr client sends no origin and it makes no difference
// for the tests here anyway.
var anyOriginHandshake = func(conf *websocket.Config, r *http.Request) error {
	return nil
}

func mustRelayConnect(url string) *Client {
	rl, err := RelayConnect(context.Background(), url)
	if err != nil {
		panic(err.Error())
	}
	return rl
}
