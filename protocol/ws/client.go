package ws

import (
	"bytes"
	"crypto/tls"
	"net/http"
	. "nostr.mleku.dev"
	"nostr.mleku.dev/codec/envelopes/closedenvelope"
	"nostr.mleku.dev/codec/envelopes/countenvelope"
	"nostr.mleku.dev/codec/envelopes/eoseenvelope"
	"nostr.mleku.dev/codec/envelopes/okenvelope"
	"nostr.mleku.dev/crypto"
	"sync"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/puzpuzpuz/xsync/v3"
	"nostr.mleku.dev/codec/envelopes"
	"nostr.mleku.dev/codec/envelopes/authenvelope"
	"nostr.mleku.dev/codec/envelopes/eventenvelope"
	"nostr.mleku.dev/codec/envelopes/noticeenvelope"
	"nostr.mleku.dev/codec/event"
	"nostr.mleku.dev/codec/filter"
	"nostr.mleku.dev/codec/filters"
	"nostr.mleku.dev/codec/kind"
	"nostr.mleku.dev/protocol/auth"
	"util.mleku.dev/atomic"
	"util.mleku.dev/context"
	"util.mleku.dev/normalize"
)

type Status int

var subscriptionIDCounter atomic.Int32

type Client struct {
	closeMutex                    sync.Mutex
	URL                           string
	RequestHeader                 http.Header // e.g. for origin header
	Connection                    *Connection
	Subscriptions                 *xsync.MapOf[string, *Subscription]
	ConnectionError               error
	connectionContext             Ctx // will be canceled when the connection closes
	connectionContextCancel       context.F
	challenge                     B      // NIP-42 challenge, we only keep the last
	notices                       chan B // NIP-01 NOTICEs
	okCallbacks                   *xsync.MapOf[string, func(bool, string)]
	writeQueue                    chan writeRequest
	subscriptionChannelCloseQueue chan *Subscription
	signatureChecker              func(*event.T) bool
	AssumeValid                   bool // this will skip verifying signatures for events received from this relay
}

type writeRequest struct {
	msg    []byte
	answer chan error
}

// NewRelay returns a new relay. The relay connection will be closed when the context is canceled.
func NewRelay(c Ctx, url S, opts ...RelayOption) *Client {
	ctx, cancel := context.Cancel(c)
	r := &Client{
		URL:                           S(normalize.URL(B(url))),
		connectionContext:             ctx,
		connectionContextCancel:       cancel,
		Subscriptions:                 xsync.NewMapOf[string, *Subscription](),
		okCallbacks:                   xsync.NewMapOf[string, func(bool, string)](),
		writeQueue:                    make(chan writeRequest),
		subscriptionChannelCloseQueue: make(chan *Subscription),
		signatureChecker:              func(e *event.T) bool { ok, _ := e.Verify(); return ok },
	}

	for _, opt := range opts {
		opt.ApplyRelayOption(r)
	}

	return r
}

// RelayConnect returns a relay object connected to url. Once successfully connected, cancelling
// ctx has no effect. To close the connection, call r.Close().
func RelayConnect(ctx Ctx, url string, opts ...RelayOption) (*Client, error) {
	r := NewRelay(context.Bg(), url, opts...)
	err := r.Connect(ctx)
	return r, err
}

// RelayOption is the type of the argument passed for that.
type RelayOption interface {
	ApplyRelayOption(*Client)
}

var (
	_ RelayOption = (WithNoticeHandler)(nil)
	_ RelayOption = (WithSignatureChecker)(nil)
)

// WithNoticeHandler just takes notices and is expected to do something with them. when not
// given, defaults to logging the notices.
type WithNoticeHandler func(notice B)

func (nh WithNoticeHandler) ApplyRelayOption(r *Client) {
	r.notices = make(chan B)
	go func() {
		for notice := range r.notices {
			nh(notice)
		}
	}()
}

// WithSignatureChecker must be a function that checks the signature of an event and returns
// true or false.
type WithSignatureChecker func(*event.T) bool

func (sc WithSignatureChecker) ApplyRelayOption(r *Client) {
	r.signatureChecker = sc
}

// String just returns the relay URL.
func (r *Client) String() string {
	return r.URL
}

// Context retrieves the context that is associated with this relay connection.
func (r *Client) Context() Ctx { return r.connectionContext }

// IsConnected returns true if the connection to this relay seems to be active.
func (r *Client) IsConnected() bool { return r.connectionContext.Err() == nil }

// Connect tries to establish a websocket connection to r.URL. If the context expires before the
// connection is complete, an error is returned. Once successfully connected, context expiration
// has no effect: call r.Close to close the connection.
//
// The underlying relay connection will use a background context. If you want to pass a custom
// context to the underlying relay connection, use NewRelay() and then Client.Connect().
func (r *Client) Connect(c Ctx) error { return r.ConnectWithTLS(c, nil) }

// ConnectWithTLS tries to establish a secured websocket connection to r.URL using customized
// tls.Config (CA's, etc).
func (r *Client) ConnectWithTLS(ctx Ctx, tlsConfig *tls.Config) error {
	if r.connectionContext == nil || r.Subscriptions == nil {
		return Errorf.E("relay must be initialized with a call to NewRelay()")
	}
	if r.URL == "" {
		return Errorf.E("invalid relay URL '%s'", r.URL)
	}
	if _, ok := ctx.Deadline(); !ok {
		// if no timeout is set, force it to 7 seconds
		var cancel context.F
		ctx, cancel = context.Timeout(ctx, 7*time.Second)
		defer cancel()
	}
	conn, err := NewConnection(ctx, r.URL, r.RequestHeader, tlsConfig)
	if err != nil {
		return Errorf.E("error opening websocket to '%s': %w", r.URL, err)
	}
	r.Connection = conn
	// ping every 29 seconds (??)
	ticker := time.NewTicker(29 * time.Second)
	// to be used when the connection is closed
	go func() {
		<-r.connectionContext.Done()
		// close these things when the connection is closed
		if r.notices != nil {
			close(r.notices)
		}
		// stop the ticker
		ticker.Stop()
		// close all subscriptions
		r.Subscriptions.Range(func(_ string, sub *Subscription) bool {
			go sub.Unsub()
			return true
		})
	}()
	// queue all write operations here so we don't do mutex spaghetti
	go func() {
		for {
			select {
			case <-ticker.C:
				err := wsutil.WriteClientMessage(r.Connection.conn, ws.OpPing, nil)
				if err != nil {
					Log.D.F("{%s} error writing ping: %v; closing websocket", r.URL,
						err)
					r.Close() // this should trigger a context cancelation
					return
				}
			case writeRequest := <-r.writeQueue:
				// all write requests will go through this to prevent races
				if err := r.Connection.WriteMessage(r.connectionContext,
					writeRequest.msg); err != nil {
					writeRequest.answer <- err
				}
				close(writeRequest.answer)
			case <-r.connectionContext.Done():
				// stop here
				return
			}
		}
	}()
	// general message reader loop
	go func() {
		buf := new(bytes.Buffer)
		for {
			buf.Reset()
			if err := conn.ReadMessage(r.connectionContext, buf); err != nil {
				r.ConnectionError = err
				r.Close()
				break
			}
			message := buf.Bytes()
			Log.D.F("{%s} %v\n", r.URL, message)

			var t S
			if t, message, err = envelopes.Identify(message); Chk.E(err) {
				continue
			}
			switch t {
			case noticeenvelope.L:
				env := noticeenvelope.New()
				if env, message, err = noticeenvelope.Parse(message); Chk.E(err) {
					continue
				}
				// see WithNoticeHandler
				if r.notices != nil {
					r.notices <- env.Message
				} else {
					Log.E.F("NOTICE from %s: '%s'\n", r.URL, env.Message)
				}
			case authenvelope.L:
				env := authenvelope.NewChallenge()
				if env, message, err = authenvelope.ParseChallenge(message); Chk.E(err) {
					continue
				}
				if len(env.Challenge) == 0 {
					continue
				}
				r.challenge = env.Challenge
			case eventenvelope.L:
				env := eventenvelope.NewResult()
				if env, message, err = eventenvelope.ParseResult(message); Chk.E(err) {
					continue
				}
				if len(env.Subscription.T) == 0 {
					continue
				}
				if sub, ok := r.Subscriptions.Load(env.Subscription.String()); !ok {
					Log.D.F("{%s} no subscription with id '%s'\n", r.URL, env.Subscription)
					continue
				} else {
					// check if the event matches the desired filter, ignore otherwise
					if !sub.Filters.Match(env.Event) {
						Log.D.F("{%s} filter does not match: %v ~ %v\n", r.URL,
							sub.Filters, env.Event)
						continue
					}
					// check signature, ignore invalid, except from trusted (AssumeValid) relays
					if !r.AssumeValid {
						if ok = r.signatureChecker(env.Event); !ok {
							Log.E.F("{%s} bad signature on %s\n", r.URL, env.Event.ID)
							continue
						}
					}
					// dispatch this to the internal .events channel of the subscription
					sub.dispatchEvent(env.Event)
				}
			case eoseenvelope.L:
				env := eoseenvelope.New()
				if env, message, err = eoseenvelope.Parse(message); Chk.E(err) {
					continue
				}
				if subscription, ok := r.Subscriptions.Load(env.Subscription.String()); ok {
					subscription.dispatchEose()
				}
			case closedenvelope.L:
				env := closedenvelope.New()
				if env, message, err = closedenvelope.Parse(message); Chk.E(err) {
					continue
				}
				if subscription, ok := r.Subscriptions.Load(env.Subscription.String()); ok {
					subscription.dispatchClosed(env.ReasonString())
				}
			case countenvelope.L:
				env := countenvelope.NewResponse()
				if env, message, err = countenvelope.Parse(message); Chk.E(err) {
					continue
				}
				if subscription, ok := r.Subscriptions.Load(env.ID.String()); ok && subscription.countResult != nil {
					subscription.countResult <- env.Count
				}
			case okenvelope.L:
				env := okenvelope.New()
				if env, message, err = okenvelope.Parse(message); Chk.E(err) {
					continue
				}
				if okCallback, exist := r.okCallbacks.Load(env.EventID.String()); exist {
					okCallback(env.OK, env.ReasonString())
				} else {
					Log.I.F("{%s} got an unexpected OK message for event %s", r.URL,
						env.EventID)
				}
			}
		}
	}()
	return nil
}

// Write queues a message to be sent to the relay.
func (r *Client) Write(msg []byte) <-chan error {
	ch := make(chan error)
	select {
	case r.writeQueue <- writeRequest{msg: msg, answer: ch}:
	case <-r.connectionContext.Done():
		go func() { ch <- Errorf.E("connection closed") }()
	}
	return ch
}

// Publish sends an "EVENT" command to the relay r as in NIP-01 and waits for an OK response.
func (r *Client) Publish(c Ctx, ev *event.T) E { return r.publish(c, ev) }

// Auth sends an "AUTH" command client->relay as in NIP-42 and waits for an OK response.
func (r *Client) Auth(c Ctx, signer crypto.Signer) error {
	authEvent := auth.CreateUnsigned(signer.Pub(), r.challenge, r.URL)
	if err := authEvent.Sign(signer); err != nil {
		return Errorf.E("error signing auth event: %w", err)
	}
	return r.publish(c, authEvent)
}

// publish can be used both for EVENT and for AUTH
func (r *Client) publish(ctx Ctx, ev *event.T) (err E) {
	var cancel context.F
	if _, ok := ctx.Deadline(); !ok {
		// if no timeout is set, force it to 7 seconds
		ctx, cancel = context.TimeoutCause(ctx, 7*time.Second,
			Errorf.E("given up waiting for an OK"))
		defer cancel()
	} else {
		// otherwise make the context cancellable so we can stop everything upon receiving an "OK"
		ctx, cancel = context.Cancel(ctx)
		defer cancel()
	}
	// listen for an OK callback
	gotOk := false
	id := ev.IDString()
	r.okCallbacks.Store(id, func(ok bool, reason string) {
		gotOk = true
		if !ok {
			err = Errorf.E("msg: %s", reason)
		}
		cancel()
	})
	defer r.okCallbacks.Delete(id)
	// publish event
	var b B
	if ev.Kind.Equal(kind.ClientAuthentication) {
		if b, err = authenvelope.NewResponseWith(ev).MarshalJSON(b); Chk.E(err) {
			return
		}
	} else {
		if b, err = eventenvelope.NewSubmissionWith(ev).MarshalJSON(b); Chk.E(err) {
			return
		}
	}
	Log.I.F("{%s} sending %s\n", r.URL, b)
	if err = <-r.Write(b); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			// this will be called when we get an OK or when the context has been canceled
			if gotOk {
				return err
			}
			return ctx.Err()
		case <-r.connectionContext.Done():
			// this is caused when we lose connectivity
			return err
		}
	}
}

// Subscribe sends a "REQ" command to the relay r as in NIP-01.
// Events are returned through the channel sub.Events.
// The subscription is closed when context ctx is cancelled ("CLOSE" in NIP-01).
//
// Remember to cancel subscriptions, either by calling `.Unsub()` on them or ensuring their `context.Context` will be canceled at some point.
// Failure to do that will result in a huge number of halted goroutines being created.
func (r *Client) Subscribe(c Ctx, ff *filters.T,
	opts ...SubscriptionOption) (*Subscription, error) {
	sub := r.PrepareSubscription(c, ff, opts...)
	if r.Connection == nil {
		return nil, Errorf.E("not connected to %s", r.URL)
	}
	if err := sub.Fire(); err != nil {
		return nil, Errorf.E("couldn't subscribe to %v at %s: %w", ff, r.URL, err)
	}
	return sub, nil
}

// PrepareSubscription creates a subscription, but doesn't fire it.
//
// Remember to cancel subscriptions, either by calling `.Unsub()` on them or ensuring their `context.Context` will be canceled at some point.
// Failure to do that will result in a huge number of halted goroutines being created.
func (r *Client) PrepareSubscription(c Ctx, ff *filters.T,
	opts ...SubscriptionOption) *Subscription {
	current := subscriptionIDCounter.Add(1)
	c, cancel := context.Cancel(c)
	sub := &Subscription{
		Relay:             r,
		Context:           c,
		cancel:            cancel,
		counter:           int(current),
		Events:            make(event.C),
		EndOfStoredEvents: make(chan struct{}, 1),
		ClosedReason:      make(chan string, 1),
		Filters:           ff,
	}
	for _, opt := range opts {
		switch o := opt.(type) {
		case WithLabel:
			sub.label = string(o)
		}
	}
	id := sub.GetID()
	r.Subscriptions.Store(id.String(), sub)
	// start handling events, eose, unsub etc:
	go sub.start()
	return sub
}

func (r *Client) QuerySync(ctx Ctx, f *filter.T,
	opts ...SubscriptionOption) ([]*event.T, error) {
	sub, err := r.Subscribe(ctx, filters.New(f), opts...)
	if err != nil {
		return nil, err
	}

	defer sub.Unsub()

	if _, ok := ctx.Deadline(); !ok {
		// if no timeout is set, force it to 7 seconds
		var cancel context.F
		ctx, cancel = context.Timeout(ctx, 7*time.Second)
		defer cancel()
	}

	var events []*event.T
	for {
		select {
		case evt := <-sub.Events:
			if evt == nil {
				// channel is closed
				return events, nil
			}
			events = append(events, evt)
		case <-sub.EndOfStoredEvents:
			return events, nil
		case <-ctx.Done():
			return events, nil
		}
	}
}

func (r *Client) Count(c Ctx, ff *filters.T, opts ...SubscriptionOption) (int, error) {
	sub := r.PrepareSubscription(c, ff, opts...)
	sub.countResult = make(chan int)

	if err := sub.Fire(); err != nil {
		return 0, err
	}

	defer sub.Unsub()

	if _, ok := c.Deadline(); !ok {
		// if no timeout is set, force it to 7 seconds
		var cancel context.F
		c, cancel = context.Timeout(c, 7*time.Second)
		defer cancel()
	}

	for {
		select {
		case count := <-sub.countResult:
			return count, nil
		case <-c.Done():
			return 0, c.Err()
		}
	}
}

func (r *Client) Close() error {
	r.closeMutex.Lock()
	defer r.closeMutex.Unlock()
	if r.connectionContextCancel == nil {
		return Errorf.E("relay already closed")
	}
	r.connectionContextCancel()
	r.connectionContextCancel = nil
	if r.Connection == nil {
		return Errorf.E("relay not connected")
	}
	err := r.Connection.Close()
	r.Connection = nil
	if err != nil {
		return err
	}
	return nil
}
