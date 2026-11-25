// Command ntfy-desktop is a lightweight ntfy desktop client.
package main

import (
	"log"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/dialog"

	"github.com/Damien-Darkh/ntfy-desktop-go/assets"
	"github.com/Damien-Darkh/ntfy-desktop-go/internal/config"
	"github.com/Damien-Darkh/ntfy-desktop-go/internal/core"
	"github.com/Damien-Darkh/ntfy-desktop-go/internal/ntfy"
	"github.com/Damien-Darkh/ntfy-desktop-go/internal/store"
	"github.com/Damien-Darkh/ntfy-desktop-go/internal/ui"
)

const appID = "io.github.damien-darkh.ntfydesktop"

func main() {
	// --- settings and database ----------------------------------------------
	cfg, err := config.Load()
	if err != nil {
		log.Printf("config: %v", err) // defaults are used; the app still starts
	}

	dir, err := config.Dir()
	if err != nil {
		log.Fatalf("config directory: %v", err)
	}
	st, err := store.Open(filepath.Join(dir, "messages.db"))
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer st.Close()

	// --- window ----------------------------------------------------------------
	a := app.NewWithID(appID)
	a.SetIcon(fyne.NewStaticResource("icon.png", assets.AppIcon))

	w := a.NewWindow("ntfy Desktop")
	w.Resize(fyne.NewSize(860, 560))
	// Hide instead of quit, so the listener keeps running in the tray.
	w.SetCloseIntercept(w.Hide)

	history := ui.NewHistoryView(w, st)
	w.SetContent(history.Content())

	// --- background pipeline: ntfy -> manager -> channel -> controller -> UI ----
	incoming := make(chan store.Message, 64)
	mgr := ntfy.NewManager(st, incoming)
	ctrl := core.New(st, incoming)

	var tray *ui.Tray

	// refreshUnread syncs the tray badge with the database.
	refreshUnread := func() {
		n, err := st.UnreadCount()
		if err != nil {
			log.Printf("unread count: %v", err)
			return
		}
		tray.SetUnread(n)
	}
	history.OnChanged = refreshUnread

	// applyConfig is the hot reload: it runs at startup and after every Save.
	applyConfig := func(c config.Config) {
		cfg = c
		if n, err := st.Prune(c.RetentionDays); err != nil {
			log.Printf("prune: %v", err)
		} else if n > 0 {
			log.Printf("pruned %d old messages", n)
		}
		history.SetSubscriptions(c.Subscriptions)
		if err := mgr.Apply(c.Subscriptions); err != nil {
			log.Printf("subscribe: %v", err)
			dialog.ShowError(err, w)
		}
		refreshUnread()
	}

	openSettings := func() { ui.ShowSettings(a, cfg, applyConfig) }

	tray = ui.SetupTray(a, w, openSettings, history.MarkEverythingRead)

	w.SetMainMenu(fyne.NewMainMenu(fyne.NewMenu("File",
		fyne.NewMenuItem("Settings…", openSettings),
		fyne.NewMenuItem("Mark all as read", history.MarkEverythingRead),
		fyne.NewMenuItem("Quit", a.Quit),
	)))

	// Called from the controller goroutine, so all UI work goes through fyne.Do.
	ctrl.OnNewMessage = func(m store.Message) {
		fyne.Do(func() {
			history.Reload()
			refreshUnread()

			title := m.Title
			if title == "" {
				title = m.Topic
			}
			// Fyne notifications carry only a title and a text; the click URL
			// and action buttons are available from the history window.
			a.SendNotification(fyne.NewNotification(title, m.Body))
		})
	}

	go mgr.Run()
	go ctrl.Run()

	a.Lifecycle().SetOnStarted(func() {
		applyConfig(cfg)
		if len(cfg.Subscriptions) == 0 {
			openSettings() // first run: ask for a topic right away
		}
	})

	// The window is shown at start so the app is usable even when no tray is
	// visible (e.g. GNOME without the AppIndicator extension).
	w.Show()
	a.Run()
}
