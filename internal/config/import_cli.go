package config

import (
	"fmt"
	"os"
	"path/filepath"

	"heckel.io/ntfy/v2/client"
)

// ImportNtfyCLI reads the ntfy command-line client's config
// (os.UserConfigDir()/ntfy/client.yml) and converts its subscriptions.
//
// Only the default host and default credentials are imported; per-topic
// credentials in client.yml are ignored. Topics with invalid names are skipped.
func ImportNtfyCLI() ([]Subscription, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(base, "ntfy", "client.yml")
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("no ntfy CLI config found at %s", path)
	}

	conf, err := client.LoadConfig(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	server := NormalizeServer(conf.DefaultHost)
	var subs []Subscription
	for _, s := range conf.Subscribe {
		sub := Subscription{Server: server, Topic: s.Topic, Token: conf.DefaultToken}
		if sub.Token == "" && conf.DefaultUser != "" {
			sub.Username = conf.DefaultUser
			if conf.DefaultPassword != nil {
				sub.Password = *conf.DefaultPassword
			}
		}
		if ValidateSubscription(sub) == nil {
			subs = append(subs, sub)
		}
	}
	if len(subs) == 0 {
		return nil, fmt.Errorf("%s contains no subscriptions", path)
	}
	return subs, nil
}
