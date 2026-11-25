package ui

import "strings"

// emojiTags maps ntfy tag short codes to emoji. ntfy shows tags that match a
// short code as emoji; any other tag is shown as plain text.
// This covers common codes only; extend it as needed.
var emojiTags = map[string]string{
	"warning": "⚠️", "rotating_light": "🚨", "white_check_mark": "✅", "heavy_check_mark": "✔️",
	"x": "❌", "no_entry": "⛔", "skull": "💀", "tada": "🎉", "partying_face": "🥳",
	"fire": "🔥", "bell": "🔔", "loudspeaker": "📢", "mega": "📣", "+1": "👍", "-1": "👎",
	"thumbsup": "👍", "thumbsdown": "👎", "package": "📦", "floppy_disk": "💾", "computer": "💻",
	"bug": "🐛", "lock": "🔒", "key": "🔑", "hourglass": "⌛", "stopwatch": "⏱️", "calendar": "📅",
	"email": "📧", "envelope": "✉️", "heart": "❤️", "star": "⭐", "rocket": "🚀", "zap": "⚡",
	"construction": "🚧", "red_circle": "🔴", "green_circle": "🟢", "information_source": "ℹ️",
	"question": "❓", "eyes": "👀", "wrench": "🔧", "hammer": "🔨", "cloud": "☁️", "snowflake": "❄️",
	"sunny": "☀️", "house": "🏠", "car": "🚗", "moneybag": "💰", "smile": "😄", "cry": "😢",
	"chart_with_upwards_trend": "📈", "chart_with_downwards_trend": "📉",
}

// SplitTags separates tags into emoji (joined with spaces) and plain-text tags.
func SplitTags(tags []string) (emojis string, rest []string) {
	var found []string
	for _, tag := range tags {
		if e, ok := emojiTags[strings.ToLower(tag)]; ok {
			found = append(found, e)
		} else {
			rest = append(rest, tag)
		}
	}
	return strings.Join(found, " "), rest
}
