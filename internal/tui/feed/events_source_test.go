package feed

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func feedEventLine(t *testing.T, actor, eventType string) string {
	t.Helper()
	b, err := json.Marshal(GtEvent{
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Source:     "gt",
		Type:       eventType,
		Actor:      actor,
		Visibility: "feed",
	})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	return string(b) + "\n"
}

func appendEventsFile(t *testing.T, path, data string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	if _, err := f.WriteString(data); err != nil {
		t.Fatalf("append %s: %v", path, err)
	}
}

func nextSourceEvent(t *testing.T, src *GtEventsSource) Event {
	t.Helper()
	select {
	case ev, ok := <-src.Events():
		if !ok {
			t.Fatal("events channel closed")
		}
		return ev
	case <-time.After(3 * time.Second):
		t.Fatal("no event within 3s")
		return Event{}
	}
}

// Events appended after the source starts must reach the channel, including
// when several polls pass with nothing new in between.
func TestGtEventsSource_StreamsEventsAppendedAfterStart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".events.jsonl")
	appendEventsFile(t, path, feedEventLine(t, "gastown/witness", "patrol_started"))

	src, err := NewGtEventsSource(dir)
	if err != nil {
		t.Fatalf("NewGtEventsSource: %v", err)
	}
	defer src.Close()

	if ev := nextSourceEvent(t, src); ev.Type != "patrol_started" {
		t.Fatalf("preloaded event type = %q, want patrol_started", ev.Type)
	}

	time.Sleep(350 * time.Millisecond) // several empty polls

	appendEventsFile(t, path, feedEventLine(t, "gastown/witness", "first_live"))
	if ev := nextSourceEvent(t, src); ev.Type != "first_live" {
		t.Fatalf("first appended event type = %q, want first_live", ev.Type)
	}

	time.Sleep(350 * time.Millisecond)

	appendEventsFile(t, path, feedEventLine(t, "gastown/refinery", "second_live"))
	if ev := nextSourceEvent(t, src); ev.Type != "second_live" {
		t.Fatalf("second appended event type = %q, want second_live", ev.Type)
	}
}

// A line written in two chunks must surface once, whole, after the second
// chunk lands.
func TestGtEventsSource_ReassemblesSplitLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".events.jsonl")
	appendEventsFile(t, path, feedEventLine(t, "gastown/witness", "preloaded"))

	src, err := NewGtEventsSource(dir)
	if err != nil {
		t.Fatalf("NewGtEventsSource: %v", err)
	}
	defer src.Close()

	// Receiving the preloaded event proves the startup scan is finished, so
	// the half line below is read by the live tail and not the preload.
	nextSourceEvent(t, src)

	line := feedEventLine(t, "gastown/witness", "split_line")
	appendEventsFile(t, path, line[:20])
	time.Sleep(350 * time.Millisecond)
	select {
	case ev := <-src.Events():
		t.Fatalf("half a line must not be emitted, got %+v", ev)
	default:
	}

	appendEventsFile(t, path, line[20:])
	ev := nextSourceEvent(t, src)
	if !strings.Contains(ev.Type, "split_line") {
		t.Fatalf("event type = %q, want split_line", ev.Type)
	}
}

// gt krc prune renames a new file over .events.jsonl. The source must keep
// delivering after that, and must not replay history the TUI already has.
func TestGtEventsSource_SurvivesFileReplacement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".events.jsonl")
	kept := feedEventLine(t, "gastown/witness", "kept")
	appendEventsFile(t, path, feedEventLine(t, "gastown/witness", "pruned")+kept)

	src, err := NewGtEventsSource(dir)
	if err != nil {
		t.Fatalf("NewGtEventsSource: %v", err)
	}
	defer src.Close()

	nextSourceEvent(t, src) // pruned (preload)
	nextSourceEvent(t, src) // kept (preload)

	time.Sleep(350 * time.Millisecond)

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(kept), 0o644); err != nil {
		t.Fatalf("write %s: %v", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		t.Fatalf("rename: %v", err)
	}
	time.Sleep(350 * time.Millisecond)

	appendEventsFile(t, path, feedEventLine(t, "gastown/refinery", "after_replace"))
	if ev := nextSourceEvent(t, src); ev.Type != "after_replace" {
		t.Fatalf("event after the replace = %q, want after_replace (retained history must not repeat)", ev.Type)
	}
}

// The model must render an event that was appended after the TUI started,
// driven the way bubbletea drives it: run the listen command, feed the
// message to Update.
func TestModel_ShowsEventAppendedAfterStart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".events.jsonl")
	appendEventsFile(t, path, feedEventLine(t, "gastown/witness", "patrol_started"))

	src, err := NewGtEventsSource(dir)
	if err != nil {
		t.Fatalf("NewGtEventsSource: %v", err)
	}
	defer src.Close()

	m := NewModel(nil)
	m.SetEventChannel(src.Events())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	deliver := func() {
		t.Helper()
		msgCh := make(chan tea.Msg, 1)
		go func() { msgCh <- m.listenForEvents()() }()
		select {
		case msg := <-msgCh:
			m.Update(msg)
		case <-time.After(3 * time.Second):
			t.Fatal("model received no event within 3s")
		}
	}

	deliver() // preloaded
	time.Sleep(350 * time.Millisecond)
	appendEventsFile(t, path, feedEventLine(t, "gastown/refinery", "merge_started"))
	deliver()

	if view := m.View(); !strings.Contains(view, "merge_started") {
		t.Fatalf("view does not show the appended event:\n%s", view)
	}
}
