# ntfy Desktop (Go + Fyne)

A lightweight ntfy client for Linux and Windows: system tray icon with an unread
badge, native desktop notifications, searchable message history, several
servers and topics.

Status: Phases 1-3 of the development plan (listener, history, settings).
Phase 4 (Do Not Disturb, quiet hours, action buttons, single instance,
packaging) is not built yet.

## Build

Prerequisites per OS are in the development plan, section 6. Then:

    go get fyne.io/fyne/v2@v2.8.0
    go get heckel.io/ntfy/v2@latest
    go get modernc.org/sqlite@latest
    go mod tidy
    go run ./cmd/ntfy-desktop

Before building, replace `yourusername` / `yourname` in `go.mod`, the import
paths, `FyneApp.toml` and `appID` in `cmd/ntfy-desktop/main.go`.

## Linux: tray icon on GNOME

Fyne's tray uses the StatusNotifier / AppIndicator protocol. KDE Plasma
supports it natively. **GNOME hides tray icons unless the "AppIndicator and
KStatusNotifierItem Support" extension is installed and enabled.** On
Ubuntu/Debian the package is usually `gnome-shell-extension-appindicator`
(confirm the name on your release); log out and back in afterwards.
Without a tray the app still works: the window opens at start.

## Where things are stored

In `os.UserConfigDir()/ntfy-desktop/`:

- `config.json`: topics, servers, tokens, retention. **Tokens and passwords
  are stored unencrypted** (file mode 0600). Revisit if you need more protection.
- `messages.db`: SQLite message history, pruned at startup and after saving
  settings (default: 30 days).

## Behaviour notes

- First run of a topic receives only new messages. After a restart or a
  dropped connection the client asks the server for everything since the last
  stored message; duplicates are dropped by message ID.
- Click URLs and action buttons live in the history window; native
  notifications only carry a title and text (a Fyne limitation).
- Connection or authentication errors are only logged to the console for now.
