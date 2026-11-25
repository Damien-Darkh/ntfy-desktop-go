package ui

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/Damien-Darkh/ntfy-desktop-go/internal/config"
)

const (
	authNone   = "No authentication"
	authBearer = "Bearer token"
	authBasic  = "Basic auth"
)

// settingsWindow is remembered so a second "Settings" click focuses the open
// window instead of opening another one.
var settingsWindow fyne.Window

// ShowSettings opens the settings window. It edits a copy of current; nothing
// changes until the user presses Save, which writes config.json and then calls
// onSave so the app can hot-reload its subscriptions.
func ShowSettings(a fyne.App, current config.Config, onSave func(config.Config)) {
	if settingsWindow != nil {
		settingsWindow.RequestFocus()
		return
	}
	win := a.NewWindow("ntfy Desktop – Settings")
	settingsWindow = win
	win.SetOnClosed(func() { settingsWindow = nil })

	draft := current
	draft.Subscriptions = append([]config.Subscription(nil), current.Subscriptions...)

	// --- list of topics, each with a remove button -------------------------
	rows := container.NewVBox()
	var rebuild func()
	rebuild = func() {
		rows.Objects = nil
		if len(draft.Subscriptions) == 0 {
			rows.Add(widget.NewLabel("No topics yet. Add one below."))
		}
		for i, s := range draft.Subscriptions {
			label := widget.NewLabel(describe(s))
			label.Truncation = fyne.TextTruncateEllipsis
			remove := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
				draft.Subscriptions = append(draft.Subscriptions[:i], draft.Subscriptions[i+1:]...)
				rebuild()
			})
			rows.Add(container.NewBorder(nil, nil, nil, remove, label))
		}
		rows.Refresh()
	}

	// --- "add a topic" form ---------------------------------------------------
	server := widget.NewEntry()
	server.SetText(config.DefaultServer)
	topic := widget.NewEntry()
	topic.SetPlaceHolder("topic name")
	token := widget.NewPasswordEntry()
	token.SetPlaceHolder("Access token (tk_…)")
	user := widget.NewEntry()
	user.SetPlaceHolder("Username")
	pass := widget.NewPasswordEntry()
	pass.SetPlaceHolder("Password")

	// Only the credential fields that match the chosen method are visible.
	auth := widget.NewSelect([]string{authNone, authBearer, authBasic}, nil)
	auth.OnChanged = func(choice string) {
		token.Hide()
		user.Hide()
		pass.Hide()
		switch choice {
		case authBearer:
			token.Show()
		case authBasic:
			user.Show()
			pass.Show()
		}
	}
	auth.SetSelected(authNone)

	add := widget.NewButtonWithIcon("Add topic", theme.ContentAddIcon(), func() {
		sub := config.Subscription{
			Server: config.NormalizeServer(server.Text),
			Topic:  strings.TrimSpace(topic.Text),
		}
		switch auth.Selected {
		case authBearer:
			sub.Token = strings.TrimSpace(token.Text)
			if sub.Token == "" {
				dialog.ShowError(errors.New("enter the access token"), win)
				return
			}
		case authBasic:
			sub.Username = strings.TrimSpace(user.Text)
			sub.Password = pass.Text
			if sub.Username == "" {
				dialog.ShowError(errors.New("enter the username"), win)
				return
			}
		}
		if err := config.ValidateSubscription(sub); err != nil {
			dialog.ShowError(err, win)
			return
		}
		if containsKey(draft.Subscriptions, sub) {
			dialog.ShowError(errors.New("this topic is already in the list"), win)
			return
		}
		draft.Subscriptions = append(draft.Subscriptions, sub)
		topic.SetText("")
		token.SetText("")
		user.SetText("")
		pass.SetText("")
		rebuild()
	})

	// --- history retention + CLI import ---------------------------------------
	retention := widget.NewEntry()
	retention.SetText(strconv.Itoa(draft.RetentionDays))

	importBtn := widget.NewButton("Import from ntfy CLI config", func() {
		subs, err := config.ImportNtfyCLI()
		if err != nil {
			dialog.ShowError(err, win)
			return
		}
		imported := 0
		for _, s := range subs {
			if !containsKey(draft.Subscriptions, s) {
				draft.Subscriptions = append(draft.Subscriptions, s)
				imported++
			}
		}
		rebuild()
		dialog.ShowInformation("Import", fmt.Sprintf("Imported %d new topic(s).", imported), win)
	})

	// --- Save / Cancel --------------------------------------------------------
	save := widget.NewButton("Save", func() {
		days, err := strconv.Atoi(strings.TrimSpace(retention.Text))
		if err != nil || days < 1 {
			dialog.ShowError(errors.New("history retention must be a whole number of days (1 or more)"), win)
			return
		}
		draft.RetentionDays = days
		if err := config.Save(draft); err != nil {
			dialog.ShowError(err, win)
			return
		}
		onSave(draft)
		win.Close()
	})
	save.Importance = widget.HighImportance
	cancel := widget.NewButton("Cancel", win.Close)

	// --- layout ---------------------------------------------------------------
	heading := func(s string) *widget.Label {
		return widget.NewLabelWithStyle(s, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	}

	topicScroll := container.NewVScroll(rows)
	topicScroll.SetMinSize(fyne.NewSize(0, 150))

	addForm := container.NewVBox(
		heading("Add a topic"),
		widget.NewForm(
			widget.NewFormItem("Server", server),
			widget.NewFormItem("Topic", topic),
			widget.NewFormItem("Authentication", auth),
		),
		token, user, pass,
		add,
	)

	note := widget.NewLabel("Tokens and passwords are stored unencrypted in config.json " +
		"(readable only by your user account).")
	note.Wrapping = fyne.TextWrapWord
	note.Importance = widget.LowImportance

	body := container.NewVBox(
		heading("Topics"), topicScroll, importBtn,
		widget.NewSeparator(),
		addForm,
		widget.NewSeparator(),
		heading("History"),
		widget.NewForm(widget.NewFormItem("Keep history (days)", retention)),
		note,
	)

	buttons := container.NewHBox(layout.NewSpacer(), cancel, save)
	win.SetContent(container.NewBorder(nil, buttons, nil, nil, container.NewVScroll(body)))
	win.Resize(fyne.NewSize(540, 640))
	rebuild()
	win.Show()
}

// describe is the one-line summary of a subscription in the settings list.
func describe(s config.Subscription) string {
	auth := "no auth"
	switch {
	case s.Token != "":
		auth = "token"
	case s.Username != "":
		auth = "user " + s.Username
	}
	return fmt.Sprintf("%s  ·  %s  ·  %s", s.Topic, hostOf(s.Server), auth)
}

func containsKey(subs []config.Subscription, s config.Subscription) bool {
	for _, x := range subs {
		if x.Key() == s.Key() {
			return true
		}
	}
	return false
}
