// Package ui holds the Fyne window and tray code.
package ui

import (
	"fmt"
	"log"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
)

// Tray is the system tray icon with its menu.
// On Linux it uses the StatusNotifier/AppIndicator protocol: KDE supports it
// natively, GNOME needs the AppIndicator extension (see README).
// All methods must be called on the UI goroutine (inside fyne.Do from others).
type Tray struct {
	app  fyne.App
	win  fyne.Window
	desk desktop.App // nil when the driver has no system tray

	normal fyne.Resource
	unread fyne.Resource
	count  int

	onSettings    func()
	onMarkAllRead func()
}

// SetupTray installs the tray icon. If the app does not run under a desktop
// driver, the returned Tray does nothing.
func SetupTray(a fyne.App, w fyne.Window, onSettings, onMarkAllRead func()) *Tray {
	t := &Tray{app: a, win: w, onSettings: onSettings, onMarkAllRead: onMarkAllRead}

	desk, ok := a.(desktop.App)
	if !ok {
		return t
	}
	t.desk = desk

	normal, unread, err := trayIcons()
	if err != nil {
		log.Printf("tray icons: %v (using a built-in icon)", err)
		normal = theme.InfoIcon()
		unread = normal
	}
	t.normal, t.unread = normal, unread

	t.apply()
	return t
}

// SetUnread updates the badge and the menu when the unread count changes.
func (t *Tray) SetUnread(n int) {
	if t.desk == nil || n == t.count {
		return
	}
	t.count = n
	t.apply()
}

// apply rebuilds the menu and sets the icon for the current unread count.
func (t *Tray) apply() {
	label := "No unread messages"
	icon := t.normal
	if t.count > 0 {
		label = fmt.Sprintf("%d unread", t.count)
		icon = t.unread
	}

	status := fyne.NewMenuItem(label, nil)
	status.Disabled = true

	menu := fyne.NewMenu("ntfy",
		status,
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Show window", func() {
			t.win.Show()
			t.win.RequestFocus()
		}),
		fyne.NewMenuItem("Mark all as read", func() {
			if t.onMarkAllRead != nil {
				t.onMarkAllRead()
			}
		}),
		fyne.NewMenuItem("Settings…", func() {
			if t.onSettings != nil {
				t.onSettings()
			}
		}),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Quit", func() {
			t.app.Quit()
		}),
	)

	t.desk.SetSystemTrayMenu(menu)
	t.desk.SetSystemTrayIcon(icon)
}
