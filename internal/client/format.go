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
	return fmt.Sprintf("%s — 🔥%d (рекорд %d) — %s", who, u.Streak, u.Best, st)
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
