package client

import (
	"path/filepath"
	"testing"

	"github.com/AlexSol/gachi-streak/internal/api"
)

func TestParseVersion(t *testing.T) {
	for s, ok := range map[string]bool{
		"0.3.0": true, "v1.20.3": true,
		"": false, "dev": false, "1.2": false, "1.2.3.4": false,
		"0.3.0-4-gabc": false, "1.02.3": false, "1.-2.3": false, "1.2.x": false,
	} {
		if _, got := ParseVersion(s); got != ok {
			t.Errorf("ParseVersion(%q) ok = %v, want %v", s, got, ok)
		}
	}
	if !versionLess("0.9.9", "0.10.0") || versionLess("1.0.0", "1.0.0") || versionLess("2.0.0", "1.9.9") {
		t.Fatal("versionLess compares numerically")
	}
}

func TestCheckUpdate(t *testing.T) {
	rel := api.ClientVersion{Latest: "0.3.0", URL: "https://gh/rel"}
	cases := []struct {
		cur, min      string
		latest        string
		want, require bool
	}{
		{cur: "0.2.1", latest: "0.3.0", want: true},
		{cur: "0.3.0", latest: "0.3.0"},
		{cur: "0.4.0", latest: "0.3.0"},
		{cur: "dev", latest: "0.3.0"},       // dev builds never update
		{cur: "0.2.1", latest: ""},          // server knows no release yet
		{cur: "0.2.1", latest: "0.3.0-rc1"}, // not a release
		{cur: "0.1.0", min: "0.2.0", latest: "0.3.0", want: true, require: true},
		{cur: "0.2.0", min: "0.2.0", latest: "0.3.0", want: true},
	}
	for _, c := range cases {
		v := rel
		v.Latest, v.Min = c.latest, c.min
		u, got := CheckUpdate(c.cur, v)
		if got != c.want || u.Required != c.require {
			t.Errorf("%s vs %+v: got %v required %v", c.cur, v, got, u.Required)
		}
	}
	if u, _ := CheckUpdate("0.1.0", api.ClientVersion{Min: "0.2.0"}); u.URL != ProjectURL+"/releases/latest" {
		t.Fatalf("fallback URL: %q", u.URL)
	}
}

func TestUpdateSeen(t *testing.T) {
	p := filepath.Join(t.TempDir(), "update_seen.txt")
	if UpdateSeen(p) != "" {
		t.Fatal("missing file = nothing seen")
	}
	if err := SetUpdateSeen(p, "0.3.0"); err != nil || UpdateSeen(p) != "0.3.0" {
		t.Fatalf("seen = %q, %v", UpdateSeen(p), err)
	}
}

func TestUpdateTexts(t *testing.T) {
	u := Update{Latest: "0.3.0"}
	if got := UpdateMenuTitle(u, "0.2.1"); got != "⬆️ Доступна нова версія v0.3.0 (у тебе v0.2.1)" {
		t.Fatalf("menu: %q", got)
	}
	if title, _ := UpdateMessage(u, "0.2.1"); title != "Gachi Streak: нова версія v0.3.0" {
		t.Fatalf("toast title: %q", title)
	}
	u.Required = true
	if _, body := UpdateMessage(u, "0.1.0"); body != "Твоя v0.1.0 більше не підтримується сервером — онови клієнт." {
		t.Fatalf("toast body: %q", body)
	}
}
