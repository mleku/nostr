package relay

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/puzpuzpuz/xsync/v3"
	"nostr.mleku.dev/codec/envelopes"
	"nostr.mleku.dev/codec/envelopes/authenvelope"
	"nostr.mleku.dev/codec/envelopes/closedenvelope"
	"nostr.mleku.dev/codec/envelopes/countenvelope"
	"nostr.mleku.dev/codec/envelopes/enveloper"
	"nostr.mleku.dev/codec/envelopes/eoseenvelope"
	"nostr.mleku.dev/codec/envelopes/eventenvelope"
	"nostr.mleku.dev/codec/envelopes/noticeenvelope"
	"nostr.mleku.dev/codec/envelopes/okenvelope"
	"nostr.mleku.dev/codec/event"
	"nostr.mleku.dev/codec/filter"
	"nostr.mleku.dev/codec/filters"
	"nostr.mleku.dev/codec/kind"
	"nostr.mleku.dev/codec/tag"
	"nostr.mleku.dev/codec/tags"
	"nostr.mleku.dev/codec/timestamp"
	"util.mleku.dev/context"
	"util.mleku.dev/normalize"
)

type Status int

var subscriptionIDCounter atomic.Int32

type Relay struct {
	closeMutex                    sync.Mutex
	URL                           string
	RequestHeader                 http.Header // e.g. for origin header
	Connection                    *Connection
	Subscriptions                 *xsync.MapOf[string, *Subscription]
	ConnectionError               error
	connectionContext             context.T // will be canceled when the connection closes
	connectionContextCancel       context.F
	challenge                     B      // NIP-42 challenge, we only keep the last
	notices                       chan B // NIP-01 NOTICEs
	okCallbacks                   *xsync.MapOf[string, func(bool, string)]
	writeQueue                    chan writeRequest
	subscriptionChannelCloseQueue chan *Subscription
	signatureChecker              func(*event.T) bool
}

type writeRequest struct {
	msg    []byte
	answer chan error
}

// NewRelay returns a new relay. The relay connection will be closed when the context is
// canceled.
func NewRelay(ctx context.T, url string, opts ...Option) *Relay {
	ctx, cancel := context.Cancel(ctx)
	r := &Relay{
		URL:                           S(normalize.URL(url)),
		connectionContext:             ctx,
		connectionContextCancel:       cancel,
		Subscriptions:                 xsync.NewMapOf[string, *Subscription](),
		okCallbacks:                   xsync.NewMapOf[string, func(bool, string)](),
		writeQueue:                    make(chan writeRequest),
		subscriptionChannelCloseQueue: make(chan *Subscription),
		signatureChecker: func(e *event.T) bool {
			ok, _ := e.Verify()
			return ok
		},
	}

	for _, opt := range opts {
		opt.ApplyRelayOption(r)
	}

	return r
}

// RelayConnect returns a relay object connected to url. Once successfully connected, cancelling
// ctx has no effect. To close the connection, call r.Close().
func RelayConnect(ctx context.T, url string, opts ...Option) (*Relay, error) {
	r := NewRelay(context.Bg(), url, opts...)
	err := r.Connect(ctx)
	return r, err
}

// Option is the type of the argument passed for that.
type Option interface {
	ApplyRelayOption(*Relay)
}

var (
	_ Option = (WithNoticeHandler)(nil)
	_ Option = (WithSignatureChecker)(nil)
)

// WithNoticeHandler just takes notices and is expected to do something with them. when not
// given, defaults to logging the notices.
type WithNoticeHandler func(notice B)

func (nh WithNoticeHandler) ApplyRelayOption(r *Relay) {
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

func (sc WithSignatureChecker) ApplyRelayOption(r *Relay) {
	r.signatureChecker = sc
}

// String just returns the relay URL.
func (r *Relay) String() string {
	return r.URL
}

// Context retrieves the context that is associated with this relay connection.
func (r *Relay) Context() context.T { return r.connectionContext }

// IsConnected returns true if the connection to this relay seems to be active.
func (r *Relay) IsConnected() bool { return r.connectionContext.Err() == nil }

// Connect tries to establish a websocket connection to r.URL. If the context expires before the
// connection is complete, an error is returned. Once successfully connected, context expiration
// has no effect: call r.Close to close the connection.
//
// The underlying relay connection will use a background context. If you want to pass a custom
// context to the underlying relay connection, use NewRelay() and then Relay.Connect().
func (r *Relay) Connect(ctx context.T) error {
	return r.ConnectWithTLS(ctx, nil)
}

// ConnectWithTLS tries to establish a secured websocket connection to r.URL using customized
// tls.Config (CA's, etc).
func (r *Relay) ConnectWithTLS(ctx context.T, tlsConfig *tls.Config) error {
	if r.connectionContext == nil || r.Subscriptions == nil {
		return fmt.Errorf("relay must be initialized with a call to NewRelay()")
	}

	if r.URL == "" {
		return fmt.Errorf("invalid relay URL '%s'", r.URL)
	}

	if _, ok := ctx.Deadline(); !ok {
		// if no timeout is set, force it to 7 seconds
		var cancel context.F
		ctx, cancel = context.Timeout(ctx, 7*time.Second)
		defer cancel()
	}

	// conn, err := NewConnection(ctx, r.URL, r.RequestHeader, tlsConfig)
	// if err != nil {
	// 	return fmt.Errorf("error opening websocket to '%s': %w", r.URL, err)
	// }
	// r.Connection = conn

	// ping every 29 seconds
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
					log.E.F("{%s} error writing ping: %v; closing websocket", r.URL,
						err)
					r.Close() // this should trigger a context cancelation
					return
				}
			case writeReq := <-r.writeQueue:
				// all write requests will go through this to prevent races
				if err := r.Connection.WriteMessage(r.connectionContext,
					writeReq.msg); err != nil {
					writeReq.answer <- err
				}
				close(writeReq.answer)
			case <-r.connectionContext.Done():
				// stop here
				return
			}
		}
	}()

	// general message reader loop
	go func() {
		buf := new(bytes.Buffer)
		var err E
		for {
			buf.Reset()
			if err = r.Connection.ReadMessage(r.connectionContext, buf); err != nil {
				r.ConnectionError = err
				r.Close()
				break
			}

			message := buf.Bytes()
			log.D.F("{%s} %v\n", r.URL, message)
			var t S
			var b B
			if t, b, err = envelopes.Identify(message); chk.E(err) {
				continue
			}
			// envelope := ParseMessage(message)
			// if envelope == nil {
			// 	continue
			// }
			switch t {
			// switch env := envelope.(type) {
			case noticeenvelope.L:
				// see WithNoticeHandler
				env := noticeenvelope.New()
				if b, err = env.MarshalJSON(b); chk.E(err) {
					continue
				}
				if r.notices != nil {
					r.notices <- env.Message
				} else {
					log.E.F("NOTICE from %s: '%s'", r.URL, env.Message)
				}
			case authenvelope.L:
				env := authenvelope.NewChallenge()
				if b, err = env.MarshalJSON(b); chk.E(err) {
					continue
				}
				if env.Challenge == nil {
					continue
				}
				r.challenge = env.Challenge
			case eventenvelope.L:
				env := eventenvelope.NewResult()
				if b, err = env.MarshalJSON(b); chk.E(err) {
					continue
				}
				if env.Subscription == nil {
					continue
				}
				if sub, ok := r.Subscriptions.Load(env.Subscription.String()); !ok {
					log.I.F("{%s} no subscription with id '%s'\n", r.URL, env.Subscription)
					continue
				} else {
					// check if the event matches the desired filter, ignore otherwise
					if !sub.Filters.Match(env.Event) {
						log.E.F("{%s} filter does not match: %v ~ %v\n", r.URL,
							sub.Filters, env.Event)
						continue
					}

					// check signature, ignore invalid
					// if ok := r.signatureChecker(env.Event.Verify()); !ok {
					if ok, err = env.Event.Verify(); chk.E(err) {
						continue
					}
					if !ok {
						log.E.F("{%s} bad signature on %s\n", r.URL, env.Event.ID)
					}
					// dispatch this to the internal .events channel of the subscription
					sub.dispatchEvent(env.Event)
				}
			case eoseenvelope.L:
				env := eoseenvelope.New()
				if b, err = env.MarshalJSON(b); chk.E(err) {
					continue
				}
				if subscription, ok := r.Subscriptions.Load(env.Subscription.String()); ok {
					subscription.dispatchEose()
				}
			case closedenvelope.L:
				env := closedenvelope.New()
				if b, err = env.MarshalJSON(b); chk.E(err) {
					continue
				}
				if subscription, ok := r.Subscriptions.Load(env.Subscription.String()); ok {
					subscription.dispatchClosed(S(env.Reason))
				}
			case countenvelope.L:
				env := countenvelope.NewResponse()
				if b, err = env.MarshalJSON(b); chk.E(err) {
					continue
				}
				if subscription, ok := r.Subscriptions.Load(env.ID.String()); ok &&
					env.Count != 0 && subscription.countResult != nil {

					subscription.countResult <- env.Count
				}
			case okenvelope.L:
				env := okenvelope.New()
				if b, err = env.MarshalJSON(b); chk.E(err) {
					continue
				}
				if okCallback, exist := r.okCallbacks.Load(env.EventID.String()); exist {
					okCallback(env.OK, S(env.Reason))
				} else {
					log.D.F("{%s} got an unexpected OK message for event %s", r.URL,
						env.EventID)
				}
			}
		}
	}()

	return nil
}

// Write queues a message to be sent to the relay.
func (r *Relay) Write(msg []byte) <-chan error {
	ch := make(chan error)
	select {
	case r.writeQueue <- writeRequest{msg: msg, answer: ch}:
	case <-r.connectionContext.Done():
		go func() { ch <- fmt.Errorf("connection closed") }()
	}
	return ch
}

// Publish sends an "EVENT" command to the relay r as in NIP-01 and waits for an OK response.
func (r *Relay) Publish(c context.T, ev *event.T) error {
	return r.publish(c, ev.ID, eventenvelope.NewSubmissionWith(ev))
}

// Auth sends an "AUTH" command client->relay as in NIP-42 and waits for an OK response.
func (r *Relay) Auth(ctx context.T, sign func(ev *event.T) E) E {
	authEvent := &event.T{
		CreatedAt: timestamp.Now(),
		Kind:      kind.ClientAuthentication,
		Tags: tags.New(tag.New("relay", r.URL),
			tag.New(B("challenge"), r.challenge)),
		Content: B{},
	}
	if err := sign(authEvent); err != nil {
		return fmt.Errorf("error signing auth event: %w", err)
	}

	return r.publish(ctx, authEvent.ID, authenvelope.NewResponseWith(authEvent))
}

// publish can be used both for EVENT and for AUTH
func (r *Relay) publish(c context.T, id B, env enveloper.I) error {
	var err error
	var cancel context.F

	if _, ok := c.Deadline(); !ok {
		// if no timeout is set, force it to 7 seconds
		c, cancel = context.TimeoutCause(c, 7*time.Second,
			fmt.Errorf("given up waiting for an OK"))
		defer cancel()
	} else {
		// otherwise make the context cancellable so we can stop everything upon receiving an "OK"
		c, cancel = context.Cancel(c)
		defer cancel()
	}

	// listen for an OK callback
	gotOk := false
	r.okCallbacks.Store(S(id), func(ok bool, reason string) {
		gotOk = true
		if !ok {
			err = fmt.Errorf("msg: %s", reason)
		}
		cancel()
	})
	defer r.okCallbacks.Delete(S(id))

	// publish event
	var envb B
	envb, _ = env.MarshalJSON(envb)
	log.D.F("{%s} sending %v\n", r.URL, envb)
	if err := <-r.Write(envb); err != nil {
		return err
	}

	for {
		select {
		case <-c.Done():
			// this will be called when we get an OK or when the context has been canceled
			if gotOk {
				return err
			}
			return c.Err()
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
// Remember to cancel subscriptions, either by calling `.Unsub()` on them or ensuring their
// `context.T` will be canceled at some point. Failure to do that will result in a huge number
// of halted goroutines being created.
func (r *Relay) Subscribe(ctx context.T, ff *filters.T,
	opts ...SubscriptionOption) (*Subscription, error) {
	sub := r.PrepareSubscription(ctx, ff, opts...)

	if r.Connection == nil {
		return nil, fmt.Errorf("not connected to %s", r.URL)
	}

	if err := sub.Fire(); err != nil {
		return nil, fmt.Errorf("couldn't subscribe to %v at %s: %w", ff, r.URL, err)
	}

	return sub, nil
}

// PrepareSubscription creates a subscription, but doesn't fire it.
//
// Remember to cancel subscriptions, either by calling `.Unsub()` on them or ensuring their
// `context.T` will be canceled at some point. Failure to do that will result in a huge number
// of halted goroutines being created.
func (r *Relay) PrepareSubscription(subC context.T, ff *filters.T,
	opts ...SubscriptionOption) *Subscription {
	current := subscriptionIDCounter.Add(1)
	subC, cancel := context.Cancel(subC)
	sub := &Subscription{
		Relay:             r,
		Context:           subC,
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
	r.Subscriptions.Store(id, sub)

	// start handling events, eose, unsub etc:
	go sub.start()

	return sub
}

func (r *Relay) QuerySync(c context.T, f *filter.T, opts ...SubscriptionOption) ([]*event.T,
	E) {
	sub, err := r.Subscribe(c, &filters.T{F: []*filter.T{f}}, opts...)
	if err != nil {
		return nil, err
	}

	defer sub.Unsub()

	if _, ok := c.Deadline(); !ok {
		// if no timeout is set, force it to 7 seconds
		var cancel context.F
		c, cancel = context.Timeout(c, 7*time.Second)
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
		case <-c.Done():
			return events, nil
		}
	}
}

func (r *Relay) Count(c context.T, ff *filters.T, opts ...SubscriptionOption) (N, E) {
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

func (r *Relay) Close() error {
	r.closeMutex.Lock()
	defer r.closeMutex.Unlock()
	if r.connectionContextCancel == nil {
		return fmt.Errorf("relay already closed")
	}
	r.connectionContextCancel()
	r.connectionContextCancel = nil
	if r.Connection == nil {
		return fmt.Errorf("relay not connected")
	}
	err := r.Connection.Close()
	r.Connection = nil
	if err != nil {
		return err
	}
	return nil
}
