//go:build windows

package winapi

import "testing"

func TestClipboardRoundTrip(t *testing.T) {
	prev, _ := ClipboardText()
	t.Cleanup(func() { _ = SetClipboardText(prev) })

	want := "📡 Мережа: 5 запитів · 2 помилок\r\nGET /status — 4"
	if err := SetClipboardText(want); err != nil {
		t.Fatal(err)
	}
	got, err := ClipboardText()
	if err != nil || got != want {
		t.Fatalf("got %q, %v", got, err)
	}
}
