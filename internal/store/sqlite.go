// Package store keeps received ntfy messages in a local SQLite database.
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver; registers itself as "sqlite"
)

// Message is one stored notification.
type Message struct {
	ID          string
	Server      string
	Topic       string
	Timestamp   int64 // Unix seconds
	Title       string
	Body        string
	Priority    int // 1 (min) .. 5 (max), 3 is the default
	Tags        []string
	ClickURL    string
	ActionsJSON string // raw JSON array, rendered as buttons in Phase 4
	Read        bool
}

// TopicKey identifies the messages of one subscription.
type TopicKey struct {
	Server string
	Topic  string
}

// Filter narrows the result of List. Zero values mean "no filter".
type Filter struct {
	Server     string
	Topic      string
	Query      string // matches title, body and topic
	UnreadOnly bool
	Limit      int // defaults to 1000
}

// Store wraps the SQLite database.
type Store struct {
	db *sql.DB
}

var schema = []string{
	`CREATE TABLE IF NOT EXISTS messages (
		id        TEXT PRIMARY KEY,
		topic     TEXT NOT NULL,
		server    TEXT NOT NULL,
		timestamp INTEGER NOT NULL,
		title     TEXT NOT NULL DEFAULT '',
		message   TEXT NOT NULL DEFAULT '',
		priority  INTEGER NOT NULL DEFAULT 3,
		tags      TEXT NOT NULL DEFAULT '[]',
		click_url TEXT NOT NULL DEFAULT '',
		actions   TEXT NOT NULL DEFAULT '[]',
		is_read   INTEGER NOT NULL DEFAULT 0
	)`,
	`CREATE INDEX IF NOT EXISTS idx_messages_topic_ts ON messages(topic, timestamp)`,
	`CREATE INDEX IF NOT EXISTS idx_messages_unread ON messages(is_read)`,
}

// Open opens (and if needed creates) the database at path.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// One connection keeps the PRAGMAs below valid for every query and avoids
	// "database is locked" errors. The load of a desktop client is tiny.
	db.SetMaxOpenConns(1)

	setup := append([]string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
	}, schema...)
	for _, stmt := range setup {
		if _, err := db.Exec(stmt); err != nil {
			db.Close()
			return nil, fmt.Errorf("initialise database: %w", err)
		}
	}
	return &Store{db: db}, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// Insert stores m. It reports false when a message with the same ID already
// exists (this happens on reconnect, when the server resends recent messages).
func (s *Store) Insert(m Message) (bool, error) {
	tags := m.Tags
	if tags == nil {
		tags = []string{}
	}
	tagsJSON, err := json.Marshal(tags)
	if err != nil {
		return false, err
	}
	actions := m.ActionsJSON
	if actions == "" || actions == "null" {
		actions = "[]"
	}
	res, err := s.db.Exec(
		`INSERT OR IGNORE INTO messages
		 (id, server, topic, timestamp, title, message, priority, tags, click_url, actions, is_read)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)`,
		m.ID, m.Server, m.Topic, m.Timestamp, m.Title, m.Body, m.Priority,
		string(tagsJSON), m.ClickURL, actions,
	)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// List returns messages newest first.
func (s *Store) List(f Filter) ([]Message, error) {
	var where []string
	var args []any
	if f.Server != "" {
		where = append(where, "server = ?")
		args = append(args, f.Server)
	}
	if f.Topic != "" {
		where = append(where, "topic = ?")
		args = append(args, f.Topic)
	}
	if f.UnreadOnly {
		where = append(where, "is_read = 0")
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		like := "%" + escapeLike(q) + "%"
		where = append(where, `(title LIKE ? ESCAPE '\' OR message LIKE ? ESCAPE '\' OR topic LIKE ? ESCAPE '\')`)
		args = append(args, like, like, like)
	}

	query := `SELECT id, server, topic, timestamp, title, message, priority, tags, click_url, actions, is_read FROM messages`
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 1000
	}
	query += " ORDER BY timestamp DESC, id DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Message
	for rows.Next() {
		var m Message
		var tags string
		var read int
		if err := rows.Scan(&m.ID, &m.Server, &m.Topic, &m.Timestamp, &m.Title, &m.Body,
			&m.Priority, &tags, &m.ClickURL, &m.ActionsJSON, &read); err != nil {
			return nil, err
		}
		m.Read = read != 0
		_ = json.Unmarshal([]byte(tags), &m.Tags) // malformed tags are simply ignored
		out = append(out, m)
	}
	return out, rows.Err()
}

// UnreadCount returns the number of unread messages across all topics.
func (s *Store) UnreadCount() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM messages WHERE is_read = 0`).Scan(&n)
	return n, err
}

// UnreadByTopic returns the unread count per subscription.
func (s *Store) UnreadByTopic() (map[TopicKey]int, error) {
	rows, err := s.db.Query(`SELECT server, topic, COUNT(*) FROM messages WHERE is_read = 0 GROUP BY server, topic`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[TopicKey]int)
	for rows.Next() {
		var k TopicKey
		var n int
		if err := rows.Scan(&k.Server, &k.Topic, &n); err != nil {
			return nil, err
		}
		out[k] = n
	}
	return out, rows.Err()
}

// MarkRead marks one message as read.
func (s *Store) MarkRead(id string) error {
	_, err := s.db.Exec(`UPDATE messages SET is_read = 1 WHERE id = ?`, id)
	return err
}

// MarkAllRead marks every unread message as read. When server and topic are
// both set, only that subscription is affected.
func (s *Store) MarkAllRead(server, topic string) error {
	if server != "" && topic != "" {
		_, err := s.db.Exec(`UPDATE messages SET is_read = 1 WHERE is_read = 0 AND server = ? AND topic = ?`, server, topic)
		return err
	}
	_, err := s.db.Exec(`UPDATE messages SET is_read = 1 WHERE is_read = 0`)
	return err
}

// LastTimestamp returns the newest stored message time for a subscription.
// ok is false when there are no messages yet.
func (s *Store) LastTimestamp(server, topic string) (ts int64, ok bool, err error) {
	var v sql.NullInt64
	err = s.db.QueryRow(`SELECT MAX(timestamp) FROM messages WHERE server = ? AND topic = ?`, server, topic).Scan(&v)
	if err != nil {
		return 0, false, err
	}
	return v.Int64, v.Valid, nil
}

// Prune deletes messages older than the given number of days and returns how
// many were removed. days <= 0 disables pruning.
func (s *Store) Prune(days int) (int64, error) {
	if days <= 0 {
		return 0, nil
	}
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour).Unix()
	res, err := s.db.Exec(`DELETE FROM messages WHERE timestamp < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// escapeLike escapes the LIKE wildcards so user input is matched literally.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
