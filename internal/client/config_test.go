package client

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSetConfigFieldsKeepsOtherKeys(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(p, []byte(`{"token":"gs_x","future_key":42,"widget_mode":"desktop"}`), 0o600)
	if err := SetConfigFields(p, map[string]any{"widget_mode": "overlay", "show_widget": false}); err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	b, _ := os.ReadFile(p)
	json.Unmarshal(b, &m)
	if m["token"] != "gs_x" || m["future_key"] != float64(42) || m["widget_mode"] != "overlay" || m["show_widget"] != false {
		t.Fatalf("config after update: %v", m)
	}
	cfg, err := LoadConfig(p, t.TempDir())
	if err != nil || cfg.NormalizedWidgetMode() != WidgetModeOverlay || cfg.ShowWidget {
		t.Fatalf("reload: %+v %v", cfg, err)
	}

	os.WriteFile(p, []byte(`{broken`), 0o600)
	if err := SetConfigFields(p, map[string]any{"x": 1}); err == nil {
		t.Fatal("must refuse to rewrite a broken config")
	}
	if b, _ := os.ReadFile(p); string(b) != "{broken" {
		t.Fatal("broken config must stay untouched")
	}
}
