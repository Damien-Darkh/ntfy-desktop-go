// Package core connects the pieces: it stores incoming messages and tells the
// UI about the ones that are new.
package core

import (
	"log"

	"github.com/Damien-Darkh/ntfy-desktop-go/internal/store"
)

// Controller reads messages from the subscription manager and persists them.
type Controller struct {
	st *store.Store
	in <-chan store.Message

	// OnNewMessage is called, from the controller's goroutine, for each message
	// that was not in the database yet. It must NOT touch Fyne objects directly:
	// wrap any UI work in fyne.Do.
	OnNewMessage func(store.Message)
}

// New creates a Controller that consumes in.
func New(st *store.Store, in <-chan store.Message) *Controller {
	return &Controller{st: st, in: in}
}

// Run processes messages until the channel is closed. Start it in a goroutine.
func (c *Controller) Run() {
	for m := range c.in {
		inserted, err := c.st.Insert(m)
		if err != nil {
			log.Printf("store message %s: %v", m.ID, err)
			continue
		}
		if !inserted {
			continue // already known, e.g. resent after a reconnect
		}
		// Phase 4: Do Not Disturb, quiet hours and the priority-5 bypass are
		// decided here, before OnNewMessage triggers a desktop notification.
		if c.OnNewMessage != nil {
			c.OnNewMessage(m)
		}
	}
}
