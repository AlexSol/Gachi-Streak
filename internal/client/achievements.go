package client

import (
	"fmt"
	"strings"

	"github.com/AlexSol/gachi-streak/internal/api"
)

// achievementNames are the Ukrainian titles of the catalog (the web keeps
// its own in frontend/src/i18n.ts; the server only knows IDs).
var achievementNames = map[string]string{
	"streak_7":   "Тиждень у залі",
	"streak_30":  "Місяць без пропуску",
	"streak_100": "Сотня",
	"streak_365": "Залізна дупа",
	"goroutine":  "Перша горутина",
	"channel":    "Перший канал",
	"select":     "Select-майстер",
	"context":    "Контекстний",
	"generics":   "Генерик",
	"errwrap":    "Обгортач помилок",
	"first_test": "Перший тест",
	"benchmark":  "Перший benchmark",
	"table_test": "Table-driven",
	"fuzz":       "Fuzz-пустун",
	"duo_7":      "Обидва в залі",
	"duo_30":     "Тренуємось разом",
	"first_fail": "Jabroni дня",
	"midnight":   "Опівнічний коміт",
	"comeback":   "Повернення",
	"night_owl":  "Нічна зміна",
	"nil_map":    "Nil-мапа",
}

// AchievementName returns the display title of an achievement.
func AchievementName(id string) string {
	if n, ok := achievementNames[id]; ok {
		return n
	}
	return id
}

// Unlock is one fresh achievement to announce.
type Unlock struct {
	ID    string
	XP    int
	Shame bool
}

// NewUnlocks returns my achievements the server flags as not yet announced,
// in catalog order.
func NewUnlocks(p api.ProgressResponse) []Unlock {
	var me *api.UserProgress
	for i := range p.Users {
		if p.Users[i].Name == p.Me {
			me = &p.Users[i]
		}
	}
	if me == nil {
		return nil
	}
	fresh := map[string]bool{}
	for _, a := range me.Achievements {
		if a.New && a.Unlocked {
			fresh[a.ID] = true
		}
	}
	var out []Unlock
	for _, d := range p.Achievements {
		if fresh[d.ID] {
			out = append(out, Unlock{ID: d.ID, XP: d.XP, Shame: d.Category == "shame"})
		}
	}
	return out
}

// UnlockMessage builds the notification title and text.
func UnlockMessage(us []Unlock) (title, body string) {
	xp := 0
	for _, u := range us {
		xp += u.XP
	}
	plus := ""
	if xp > 0 {
		plus = fmt.Sprintf(" · +%d XP", xp)
	}
	if len(us) == 1 {
		title = "🏆 Ачівку відкрито!"
		if us[0].Shame {
			title = "💀 Ганебна ачівка"
		}
		return title, AchievementName(us[0].ID) + plus
	}
	names := make([]string, 0, 3)
	for i, u := range us {
		if i == 3 {
			names = append(names, fmt.Sprintf("і ще %d", len(us)-3))
			break
		}
		names = append(names, AchievementName(u.ID))
	}
	return fmt.Sprintf("🏆 Відкрито ачівок: %d", len(us)), strings.Join(names, ", ") + plus
}

// AchievementCount returns how many of the catalog I have unlocked.
func AchievementCount(p api.ProgressResponse) (unlocked, total int) {
	for _, u := range p.Users {
		if u.Name != p.Me {
			continue
		}
		for _, a := range u.Achievements {
			if a.Unlocked {
				unlocked++
			}
		}
	}
	return unlocked, len(p.Achievements)
}
