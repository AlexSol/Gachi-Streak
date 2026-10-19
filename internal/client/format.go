package client

import (
	"fmt"
	"time"

	"github.com/AlexSol/gachi-streak/internal/api"
)

// FormatLeft renders a duration like "4г 05хв".
func FormatLeft(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	return fmt.Sprintf("%dг %02dхв", h, m)
}

// StatusLine is a one-line tray menu entry for a user.
func StatusLine(u api.UserStatus, isMe bool) string {
	who := u.Name
	if isMe {
		who += " (ти)"
	}
	left := FormatLeft(time.Duration(u.SecondsLeft) * time.Second)
	var st string
	switch u.State {
	case "safe":
		st = "✅ сьогодні зроблено"
	case "pending":
		st = "⏳ лишилось " + left
	case "warning":
		st = "⚠️ ГОРИТЬ! лишилось " + left
	case "failed":
		st = "💀 стрик злитий, лишилось " + left
	default:
		st = u.State
	}
	line := fmt.Sprintf("%s — 🔥%d (рекорд %d) — %s", who, u.Streak, u.Best, st)
	if p := PresenceText(u.Presence, time.Now()); p != "" {
		line += " · " + p
	}
	return line
}

// PresenceText is a Discord-like "now in GoLand · 25хв" string.
func PresenceText(p *api.Presence, now time.Time) string {
	switch {
	case p == nil || !p.Online:
		return ""
	case p.Idle:
		return "💤 AFK"
	case p.App == "":
		return ""
	case p.Since.IsZero():
		return "🖥 " + p.App
	default:
		return "🖥 " + p.App + " " + FormatDuration(now.Sub(p.Since))
	}
}

// FormatDuration renders a duration like "1г 05хв" or "25хв".
func FormatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h == 0 {
		return fmt.Sprintf("%dхв", m)
	}
	return fmt.Sprintf("%dг %02dхв", h, m)
}

// Tooltip is the tray hover text (Windows limits it to ~127 chars).
func Tooltip(me api.UserStatus) string {
	switch me.State {
	case "safe":
		return fmt.Sprintf("Gachi Streak: 🔥%d — на сьогодні все", me.Streak)
	case "failed":
		return "Gachi Streak: 💀 стрик злитий. Сідай за Go."
	default:
		return fmt.Sprintf("Gachi Streak: 🔥%d — до дедлайну %s", me.Streak,
			FormatLeft(time.Duration(me.SecondsLeft)*time.Second))
	}
}
