package ui

import (
	"fmt"
	"image/color"
	"log"
	"net/url"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/Damien-Darkh/ntfy-desktop-go/internal/config"
	"github.com/Damien-Darkh/ntfy-desktop-go/internal/store"
)

// HistoryView is the main window content: a topic sidebar on the left and a
// searchable message list on the right.
//
// widget.List is virtualized: only the visible rows exist as widgets, so a
// long history costs little memory.
//
// All methods must be called on the UI goroutine (inside fyne.Do from others).
type HistoryView struct {
	// OnChanged is called after the read state changed, so the tray can update.
	OnChanged func()

	win fyne.Window
	st  *store.Store

	subs     []config.Subscription
	selected *store.TopicKey // nil means "all topics"
	msgs     []store.Message // what the list currently shows
	unread   map[store.TopicKey]int

	search     *widget.Entry
	unreadOnly *widget.Check
	empty      *widget.Label
	list       *widget.List
	sidebar    *widget.List
	content    fyne.CanvasObject
}

// NewHistoryView builds the view. Call SetSubscriptions once the config is known.
func NewHistoryView(win fyne.Window, st *store.Store) *HistoryView {
	h := &HistoryView{win: win, st: st, unread: make(map[store.TopicKey]int)}

	h.search = widget.NewEntry()
	h.search.SetPlaceHolder("Search messages…")
	h.search.OnChanged = func(string) { h.Reload() }

	h.unreadOnly = widget.NewCheck("Unread only", func(bool) { h.Reload() })

	h.empty = widget.NewLabel("")
	h.empty.Alignment = fyne.TextAlignCenter

	// Message list.
	h.list = widget.NewList(
		func() int { return len(h.msgs) },
		func() fyne.CanvasObject { return newMessageRow() },
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			if id < len(h.msgs) {
				obj.(*messageRow).set(h.msgs[id])
			}
		},
	)
	h.list.OnSelected = func(id widget.ListItemID) {
		h.list.UnselectAll() // so the same message can be clicked again
		h.openMessage(id)
	}

	// Topic sidebar: row 0 is "All topics", then one row per subscription.
	h.sidebar = widget.NewList(
		func() int { return len(h.subs) + 1 },
		func() fyne.CanvasObject {
			l := widget.NewLabel("a fairly long topic name")
			l.Truncation = fyne.TextTruncateEllipsis
			return l
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			obj.(*widget.Label).SetText(h.sidebarLabel(id))
		},
	)
	h.sidebar.OnSelected = func(id widget.ListItemID) {
		if id == 0 || id-1 >= len(h.subs) {
			h.selected = nil
		} else {
			s := h.subs[id-1]
			h.selected = &store.TopicKey{Server: s.Server, Topic: s.Topic}
		}
		h.Reload()
	}

	markRead := widget.NewButtonWithIcon("Mark all read", theme.ConfirmIcon(), h.MarkAllRead)
	top := container.NewBorder(nil, nil, nil, container.NewHBox(h.unreadOnly, markRead), h.search)
	main := container.NewBorder(top, nil, nil, nil,
		container.NewStack(h.list, container.NewCenter(h.empty)))

	split := container.NewHSplit(h.sidebar, main)
	split.Offset = 0.26
	h.content = split

	h.sidebar.Select(0)
	return h
}

// Content returns the widget tree to put into the window.
func (h *HistoryView) Content() fyne.CanvasObject { return h.content }

// SetSubscriptions updates the sidebar after the config changed. The filter
// goes back to "All topics" so the selection can never point at a removed topic.
func (h *HistoryView) SetSubscriptions(subs []config.Subscription) {
	h.subs = subs
	h.selected = nil
	h.sidebar.Refresh()
	h.sidebar.Select(0)
	h.Reload()
}

// Reload re-queries the database using the current search text and filters.
func (h *HistoryView) Reload() {
	f := store.Filter{Query: h.search.Text, UnreadOnly: h.unreadOnly.Checked, Limit: 1000}
	if h.selected != nil {
		f.Server, f.Topic = h.selected.Server, h.selected.Topic
	}
	msgs, err := h.st.List(f)
	if err != nil {
		log.Printf("load history: %v", err)
		return
	}
	h.msgs = msgs
	h.list.Refresh()

	if len(msgs) == 0 {
		h.empty.SetText(h.emptyText())
		h.empty.Show()
	} else {
		h.empty.Hide()
	}
	h.RefreshCounts()
}

// RefreshCounts updates the unread numbers in the sidebar.
func (h *HistoryView) RefreshCounts() {
	counts, err := h.st.UnreadByTopic()
	if err != nil {
		log.Printf("unread counts: %v", err)
		return
	}
	h.unread = counts
	h.sidebar.Refresh()
}

// MarkAllRead marks the messages of the selected topic (or all topics) as read.
func (h *HistoryView) MarkAllRead() {
	var server, topic string
	if h.selected != nil {
		server, topic = h.selected.Server, h.selected.Topic
	}
	h.markRead(server, topic)
}

// MarkEverythingRead marks every message as read, whatever is selected.
func (h *HistoryView) MarkEverythingRead() { h.markRead("", "") }

func (h *HistoryView) markRead(server, topic string) {
	if err := h.st.MarkAllRead(server, topic); err != nil {
		dialog.ShowError(err, h.win)
		return
	}
	h.Reload()
	h.changed()
}

func (h *HistoryView) changed() {
	if h.OnChanged != nil {
		h.OnChanged()
	}
}

// openMessage marks the message as read and shows its full text.
func (h *HistoryView) openMessage(id widget.ListItemID) {
	if id < 0 || id >= len(h.msgs) {
		return
	}
	m := h.msgs[id]

	if !m.Read {
		if err := h.st.MarkRead(m.ID); err != nil {
			log.Printf("mark read: %v", err)
		} else {
			h.msgs[id].Read = true
			h.list.RefreshItem(id)
			h.RefreshCounts()
			h.changed()
		}
	}

	emojis, rest := SplitTags(m.Tags)

	title := m.Title
	if title == "" {
		title = m.Topic
	}
	if emojis != "" {
		title = emojis + " " + title
	}

	meta := fmt.Sprintf("%s · %s · priority %d",
		m.Topic, time.Unix(m.Timestamp, 0).Format("02 Jan 2006 15:04"), m.Priority)
	if len(rest) > 0 {
		meta += " · " + strings.Join(rest, ", ")
	}
	metaLabel := widget.NewLabel(meta)
	metaLabel.Importance = widget.LowImportance
	metaLabel.Wrapping = fyne.TextWrapWord

	body := widget.NewLabel(m.Body)
	body.Wrapping = fyne.TextWrapWord
	scroll := container.NewVScroll(body)

	var content *fyne.Container
	if m.ClickURL != "" {
		link := widget.NewButton("Open link", func() { h.openURL(m.ClickURL) })
		content = container.NewBorder(metaLabel, link, nil, nil, scroll)
	} else {
		content = container.NewBorder(metaLabel, nil, nil, nil, scroll)
	}

	d := dialog.NewCustom(title, "Close", content, h.win)
	d.Resize(fyne.NewSize(480, 320))
	d.Show()
}

// openURL opens a message's click URL in the default browser or mail client.
// Only http, https and mailto are allowed; a notification must not be able to
// launch arbitrary local handlers.
func (h *HistoryView) openURL(raw string) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "mailto") {
		dialog.ShowInformation("Link not opened",
			"Only http, https and mailto links can be opened from here:\n"+raw, h.win)
		return
	}
	if err := fyne.CurrentApp().OpenURL(u); err != nil {
		dialog.ShowError(err, h.win)
	}
}

func (h *HistoryView) emptyText() string {
	switch {
	case len(h.subs) == 0:
		return "No topics yet.\nOpen File → Settings to add one."
	case h.search.Text != "" || h.unreadOnly.Checked:
		return "No messages match."
	default:
		return "No messages yet."
	}
}

func (h *HistoryView) sidebarLabel(id int) string {
	if id == 0 {
		total := 0
		for _, n := range h.unread {
			total += n
		}
		return withCount("All topics", total)
	}
	if id-1 >= len(h.subs) {
		return ""
	}
	s := h.subs[id-1]
	name := s.Topic
	if h.multiServer() {
		name += " (" + hostOf(s.Server) + ")"
	}
	return withCount(name, h.unread[store.TopicKey{Server: s.Server, Topic: s.Topic}])
}

// multiServer reports whether the subscriptions span more than one server, in
// which case the sidebar shows the server name next to the topic.
func (h *HistoryView) multiServer() bool {
	for _, s := range h.subs {
		if s.Server != h.subs[0].Server {
			return true
		}
	}
	return false
}

func withCount(name string, n int) string {
	if n > 0 {
		return fmt.Sprintf("%s  (%d)", name, n)
	}
	return name
}

func hostOf(server string) string {
	if u, err := url.Parse(server); err == nil && u.Host != "" {
		return u.Host
	}
	return server
}

// ---------------------------------------------------------------------------
// One row of the message list.
// ---------------------------------------------------------------------------

// messageRow is a custom widget: a coloured bar on the left (priority / unread)
// and three lines of text. Unread messages have a bold title.
type messageRow struct {
	widget.BaseWidget
	bar   *canvas.Rectangle
	title *widget.Label
	body  *widget.Label
	meta  *widget.Label
}

func newMessageRow() *messageRow {
	r := &messageRow{
		bar:   canvas.NewRectangle(color.Transparent),
		title: widget.NewLabel(""),
		body:  widget.NewLabel(""),
		meta:  widget.NewLabel(""),
	}
	r.bar.SetMinSize(fyne.NewSize(6, 0))
	r.title.Truncation = fyne.TextTruncateEllipsis
	r.body.Truncation = fyne.TextTruncateEllipsis
	r.meta.Truncation = fyne.TextTruncateEllipsis
	r.meta.Importance = widget.LowImportance
	r.ExtendBaseWidget(r)
	return r
}

// CreateRenderer implements fyne.Widget.
func (r *messageRow) CreateRenderer() fyne.WidgetRenderer {
	text := container.NewVBox(r.title, r.body, r.meta)
	return widget.NewSimpleRenderer(container.NewBorder(nil, nil, r.bar, nil, text))
}

// set fills the row from a stored message.
func (r *messageRow) set(m store.Message) {
	emojis, rest := SplitTags(m.Tags)

	title := m.Title
	if title == "" {
		title = m.Topic
	}
	if emojis != "" {
		title = emojis + " " + title
	}
	r.title.TextStyle = fyne.TextStyle{Bold: !m.Read}
	r.title.SetText(title)
	r.title.Refresh()

	// One-line preview: newlines and repeated spaces collapsed.
	r.body.SetText(strings.Join(strings.Fields(m.Body), " "))

	meta := m.Topic + " · " + timeLabel(m.Timestamp)
	if len(rest) > 0 {
		meta += " · " + strings.Join(rest, ", ")
	}
	r.meta.SetText(meta)

	r.bar.FillColor = priorityColor(m.Priority, m.Read)
	r.bar.Refresh()
}

// priorityColor picks the colour of the row's left bar. High priorities are
// always coloured; normal and low ones only show a bar while unread.
func priorityColor(priority int, read bool) color.Color {
	switch {
	case priority >= 5:
		return color.NRGBA{R: 0xD3, G: 0x2F, B: 0x2F, A: 0xFF} // red
	case priority == 4:
		return color.NRGBA{R: 0xF5, G: 0x7C, B: 0x00, A: 0xFF} // orange
	case read:
		return color.Transparent
	case priority <= 2:
		return color.NRGBA{R: 0x9E, G: 0x9E, B: 0x9E, A: 0xFF} // grey
	default:
		return color.NRGBA{R: 0x1E, G: 0x88, B: 0xE5, A: 0xFF} // blue
	}
}

// timeLabel shows only the time for today, the date and time for this year,
// and just the date for older messages.
func timeLabel(ts int64) string {
	t := time.Unix(ts, 0)
	now := time.Now()
	switch {
	case t.Year() == now.Year() && t.YearDay() == now.YearDay():
		return t.Format("15:04")
	case t.Year() == now.Year():
		return t.Format("02 Jan 15:04")
	default:
		return t.Format("02 Jan 2006")
	}
}
