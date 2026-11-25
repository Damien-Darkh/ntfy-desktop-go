// Package ntfy wraps the official ntfy Go client.
package ntfy

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"heckel.io/ntfy/v2/client"

	"github.com/Damien-Darkh/ntfy-desktop-go/internal/config"
	"github.com/Damien-Darkh/ntfy-desktop-go/internal/store"
)

type activeSub struct {
	id  string // subscription ID handed out by client.Subscribe
	sub config.Subscription
}

// Manager owns ONE ntfy client. Every subscription (any server, any topic) is
// started on it with a full topic URL, and all messages arrive on the client's
// single Messages channel, which Run forwards.
type Manager struct {
	c   *client.Client
	st  *store.Store
	out chan<- store.Message

	mu     sync.Mutex
	active map[string]activeSub           // Subscription.Key() -> running subscription
	byID   map[string]config.Subscription // subscription ID -> subscription
}

// NewManager creates a Manager. Incoming messages are converted and sent to out.
func NewManager(st *store.Store, out chan<- store.Message) *Manager {
	return &Manager{
		c:      client.New(client.NewConfig()),
		st:     st,
		out:    out,
		active: make(map[string]activeSub),
		byID:   make(map[string]config.Subscription),
	}
}

// Apply makes the running subscriptions match subs: removed or changed ones
// are stopped, new or changed ones are started. Safe to call repeatedly
// (hot reload after the settings change).
func (m *Manager) Apply(subs []config.Subscription) error {
	desired := make(map[string]config.Subscription, len(subs))
	for _, s := range subs {
		desired[s.Key()] = s
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Stop what is gone or whose credentials changed.
	for key, a := range m.active {
		if want, ok := desired[key]; !ok || want != a.sub {
			m.c.Unsubscribe(a.id)
			delete(m.byID, a.id)
			delete(m.active, key)
		}
	}

	// Start what is new.
	var errs []error
	for key, s := range desired {
		if _, running := m.active[key]; running {
			continue
		}
		id, err := m.c.Subscribe(s.URL(), m.options(s)...)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", s.URL(), err))
			continue
		}
		m.active[key] = activeSub{id: id, sub: s}
		m.byID[id] = s
	}
	return errors.Join(errs...)
}

// options builds the per-subscription request options.
//
// since: the client's reconnect loop is crude (it retries every 10 seconds with
// the SAME options), so we always pass a since timestamp. That way every
// reconnect asks the server for what was missed. It is the time of the newest
// stored message or, for a topic we have never seen, "now" - so a first run
// receives only new messages. Duplicates are dropped by the store (message ID).
func (m *Manager) options(s config.Subscription) []client.SubscribeOption {
	since := time.Now().Unix()
	if ts, ok, err := m.st.LastTimestamp(s.Server, s.Topic); err == nil && ok {
		since = ts
	}

	opts := []client.SubscribeOption{client.WithSinceUnixTime(since)}
	switch {
	case s.Token != "":
		opts = append(opts, client.WithBearerAuth(s.Token))
	case s.Username != "":
		opts = append(opts, client.WithBasicAuth(s.Username, s.Password))
	}
	return opts
}

// Run forwards every incoming message to the out channel. Start it in its own
// goroutine; it returns only if the client's channel is closed.
func (m *Manager) Run() {
	for msg := range m.c.Messages {
		m.mu.Lock()
		sub, ok := m.byID[msg.SubscriptionID]
		m.mu.Unlock()
		if !ok {
			continue // subscription was removed while the message was in flight
		}
		m.out <- toStoreMessage(sub, msg)
	}
}

func toStoreMessage(sub config.Subscription, msg *client.Message) store.Message {
	prio := msg.Priority
	if prio == 0 {
		prio = 3 // the server omits the default priority
	}
	return store.Message{
		ID:          msg.ID,
		Server:      sub.Server,
		Topic:       sub.Topic,
		Timestamp:   msg.Time,
		Title:       msg.Title,
		Body:        msg.Message,
		Priority:    prio,
		Tags:        msg.Tags,
		ClickURL:    msg.Click,
		ActionsJSON: actionsFromRaw(msg.Raw),
	}
}

// actionsFromRaw pulls the "actions" array out of the raw JSON line.
// client.Message has no typed field for it, so we read it from Raw and keep it
// as JSON until Phase 4 renders the buttons.
func actionsFromRaw(raw string) string {
	var envelope struct {
		Actions json.RawMessage `json:"actions"`
	}
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil || len(envelope.Actions) == 0 {
		return "[]"
	}
	return string(envelope.Actions)
}
