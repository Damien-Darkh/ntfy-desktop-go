// Package config loads and saves the application settings.
//
// Settings live in os.UserConfigDir()/ntfy-desktop/config.json.
// Tokens and passwords are stored in plain text for now (file mode 0600).
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	// DefaultServer is used when a server field is left empty.
	DefaultServer = "https://ntfy.sh"
	// DefaultRetentionDays is how long messages are kept.
	DefaultRetentionDays = 30

	dirName  = "ntfy-desktop"
	fileName = "config.json"
)

// ntfy only accepts these topic names (same rule as the server).
var topicRegex = regexp.MustCompile(`^[-_A-Za-z0-9]{1,64}$`)

// Subscription is one topic on one server, with optional credentials.
// Use either Token (Bearer) or Username/Password (Basic), not both.
type Subscription struct {
	Server   string `json:"server"`
	Topic    string `json:"topic"`
	Token    string `json:"token,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

// Key identifies a subscription regardless of its credentials.
func (s Subscription) Key() string { return s.Server + "|" + s.Topic }

// URL is the full topic URL, e.g. https://ntfy.sh/mytopic.
func (s Subscription) URL() string { return strings.TrimRight(s.Server, "/") + "/" + s.Topic }

// Config is the whole settings file.
type Config struct {
	Subscriptions []Subscription `json:"subscriptions"`
	RetentionDays int            `json:"retention_days"`
}

// Dir returns the application's config directory, creating it if needed.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, dirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// Load reads the config file. A missing file is not an error: defaults are
// returned. A damaged file is renamed to config.json.bad and defaults are
// returned together with an error, so a later Save cannot silently overwrite it.
func Load() (Config, error) {
	cfg := Config{RetentionDays: DefaultRetentionDays}
	dir, err := Dir()
	if err != nil {
		return cfg, err
	}
	path := filepath.Join(dir, fileName)

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		_ = os.Rename(path, path+".bad")
		return Config{RetentionDays: DefaultRetentionDays},
			fmt.Errorf("config.json is damaged (moved to config.json.bad): %w", err)
	}
	if cfg.RetentionDays <= 0 {
		cfg.RetentionDays = DefaultRetentionDays
	}
	for i := range cfg.Subscriptions {
		cfg.Subscriptions[i].Server = NormalizeServer(cfg.Subscriptions[i].Server)
	}
	return cfg, nil
}

// Save writes the config atomically (temp file, then rename).
func Save(cfg Config) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, fileName)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// NormalizeServer trims the input, adds https:// when no scheme is given,
// removes a trailing slash and falls back to DefaultServer when empty.
func NormalizeServer(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return DefaultServer
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	return strings.TrimRight(s, "/")
}

// ValidateSubscription checks the topic name and the server URL.
func ValidateSubscription(s Subscription) error {
	if !topicRegex.MatchString(s.Topic) {
		return errors.New("topic names may only contain letters, digits, '-' and '_' (at most 64 characters)")
	}
	u, err := url.Parse(s.Server)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("invalid server URL %q", s.Server)
	}
	return nil
}
