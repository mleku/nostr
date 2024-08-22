package relayws

import (
	"crypto/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"ec.mleku.dev/v2/bech32"
	w "github.com/fasthttp/websocket"
	"nostr.mleku.dev/codec/bech32encoding"
	"util.mleku.dev/atomic"
	"util.mleku.dev/context"
	"util.mleku.dev/qu"
)

type MessageType int

// WS is a wrapper around a fasthttp/websocket with mutex locking and NIP-42 IsAuthed support
type WS struct {
	Conn         *w.Conn
	remote       atomic.String
	mutex        sync.Mutex
	Request      *http.Request // original request
	challenge    atomic.String // nip42
	Pending      atomic.Value  // for DM CLI authentication
	authPub      atomic.Value
	Authed       qu.C
	OffenseCount atomic.Uint32 // when client does dumb stuff, increment this
	Ctx          context.T
}

func New(c context.T, conn *w.Conn, r *http.Request, maxMsg int) (ws *WS) {
	// authPubKey must be initialized with a zero length slice so it can be detected when it
	// hasn't been loaded.
	var authPubKey atomic.Value
	authPubKey.Store(B{})
	ws = &WS{Conn: conn, Request: r, Authed: qu.T(), authPub: authPubKey}
	ws.generateChallenge()
	ws.setRemote(r)
	conn.SetReadLimit(int64(maxMsg))
	conn.EnableWriteCompression(true)
	ws.Ctx = c
	return
}

func (ws *WS) Ping() (err E) { return ws.write(w.PingMessage, nil) }
func (ws *WS) Pong() (err E) { return ws.write(w.PongMessage, nil) }

// write writes a message with a given websocket type specifier
func (ws *WS) write(t MessageType, b B) (err E) {
	ws.mutex.Lock()
	defer ws.mutex.Unlock()
	if len(b) != 0 {
		log.T.F("sending message to %s %0x\n%s", ws.Remote(), ws.AuthPub(), string(b))
	}
	chk.E(ws.Conn.SetWriteDeadline(time.Now().Add(time.Second * 5)))
	return ws.Conn.WriteMessage(int(t), b)
}

// WriteTextMessage writes a text (binary?) message
func (ws *WS) WriteTextMessage(b B) (err E) {
	return ws.write(w.TextMessage, b)
}

const ChallengeLength = 16
const ChallengeHRP = "nchal"

// generateChallenge gathers new entropy to generate a new challenge, stores it and returns it.
func (ws *WS) generateChallenge() (challenge S) {
	var err error
	// create a new challenge for this connection
	cb := make([]byte, ChallengeLength)
	if _, err = rand.Read(cb); chk.E(err) {
		// i never know what to do for this case, panic? usually just ignore, it should never happen
		panic(err)
	}
	var b5 B
	if b5, err = bech32encoding.ConvertForBech32(cb); chk.E(err) {
		return
	}
	var encoded B
	if encoded, err = bech32.Encode(bech32.B(ChallengeHRP), b5); chk.E(err) {
		return
	}
	challenge = S(encoded)
	ws.challenge.Store(challenge)
	return
}

// Challenge returns the current challenge on a websocket.
func (ws *WS) Challenge() (challenge B) { return B(ws.challenge.Load()) }

// Remote returns the current real remote.
func (ws *WS) Remote() (remote S) { return ws.remote.Load() }
func (ws *WS) SetRemote(remote S) { ws.remote.Store(remote) }

// SetAuthPub loads the authPubKey atomic of the websocket.
func (ws *WS) SetAuthPub(a B) {
	aa := make(B, 0, len(a))
	copy(aa, a)
	ws.authPub.Store(aa)
}

// AuthPub returns the current authed Pubkey.
func (ws *WS) AuthPub() (a B) {
	b := ws.authPub.Load().(B)
	a = make(B, 0, len(b))
	// make a copy because bytes are references
	a = append(a, b...)
	return
}

func (ws *WS) HasAuth() bool {
	b := ws.authPub.Load().(B)
	return len(b) > 0
}

func (ws *WS) setRemote(r *http.Request) {
	var rr string
	// reverse proxy should populate this field so we see the remote not the proxy
	rem := r.Header.Get("X-Forwarded-For")
	if rem != "" {
		splitted := strings.Split(rem, " ")
		if len(splitted) == 1 {
			rr = splitted[0]
		}
		if len(splitted) == 2 {
			rr = splitted[1]
		}
		// in case upstream doesn't set this or we are directly listening instead of
		// via reverse proxy or just if the header field is missing, put the
		// connection remote address into the websocket state data.
		if rr == "" {
			rr = r.RemoteAddr
		}
	} else {
		// if that fails, fall back to the remote (probably the proxy, unless the relay is
		// actually directly listening)
		rr = ws.Conn.NetConn().RemoteAddr().String()
	}
	ws.SetRemote(rr)
}
