package client

import (
	"testing"

	"github.com/AlexSol/gachi-streak/internal/api"
	"github.com/AlexSol/gachi-streak/internal/progress"
)

func TestEveryAchievementHasAName(t *testing.T) {
	for _, d := range progress.Achievements {
		if AchievementName(d.ID) == d.ID {
			t.Errorf("no name for %s", d.ID)
		}
	}
}

func TestNewUnlocks(t *testing.T) {
	p := api.ProgressResponse{
		Me: "alex",
		Achievements: []api.AchievementDef{
			{ID: "streak_7", XP: 30, Category: "streak"},
			{ID: "first_fail", XP: 0, Category: "shame"},
			{ID: "goroutine", XP: 40, Category: "go"},
		},
		Users: []api.UserProgress{
			{Name: "sasha", Achievements: []api.AchievementState{{ID: "goroutine", Unlocked: true, New: true}}},
			{Name: "alex", Achievements: []api.AchievementState{
				{ID: "goroutine", Unlocked: true, New: true},
				{ID: "first_fail", Unlocked: true, New: true},
				{ID: "streak_7", Unlocked: true},
			}},
		},
	}
	got := NewUnlocks(p)
	if len(got) != 2 || got[0].ID != "first_fail" || !got[0].Shame || got[1].ID != "goroutine" {
		t.Fatalf("got %+v", got)
	}
	if u, total := AchievementCount(p); u != 3 || total != 3 {
		t.Fatalf("count %d/%d", u, total)
	}
	p.Me = "nobody"
	if NewUnlocks(p) != nil {
		t.Fatal("unknown me")
	}
}

func TestUnlockMessage(t *testing.T) {
	title, body := UnlockMessage([]Unlock{{ID: "goroutine", XP: 40}})
	if title != "🏆 Ачівку відкрито!" || body != "Перша горутина · +40 XP" {
		t.Fatalf("%q %q", title, body)
	}
	title, body = UnlockMessage([]Unlock{{ID: "first_fail", Shame: true}})
	if title != "💀 Ганебна ачівка" || body != "Jabroni дня" {
		t.Fatalf("%q %q", title, body)
	}
	title, body = UnlockMessage([]Unlock{{ID: "streak_7", XP: 30}, {ID: "channel", XP: 40}, {ID: "select", XP: 50}, {ID: "fuzz", XP: 80}, {ID: "x", XP: 0}})
	if title != "🏆 Відкрито ачівок: 5" || body != "Тиждень у залі, Перший канал, Select-майстер, і ще 2 · +200 XP" {
		t.Fatalf("%q %q", title, body)
	}
}
