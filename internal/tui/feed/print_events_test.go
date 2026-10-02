package feed

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// writeTestEvents writes GtEvent JSON lines to a temporary .events.jsonl file
// and returns the directory path (townRoot).
func writeTestEvents(t *testing.T, events []GtEvent) string {
	t.Helper()
	dir := t.TempDir()
	eventsPath := filepath.Join(dir, ".events.jsonl")
	var lines []string
	for _, ev := range events {
		b, err := json.Marshal(ev)
		if err != nil {
			t.Fatalf("marshal event: %v", err)
		}
		lines = append(lines, string(b))
	}
	if err := os.WriteFile(eventsPath, []byte(strings.Join(lines, "\n")+"\n"), 0644); err != nil {
		t.Fatalf("write events file: %v", err)
	}
	return dir
}

func TestPrintGtEvents_ReadsAndFormats(t *testing.T) {
	now := time.Now()
	townRoot := writeTestEvents(t, []GtEvent{
		{Timestamp: now.Add(-2 * time.Minute).Format(time.RFC3339), Source: "test", Type: "create", Actor: "gastown/witness", Visibility: "feed", Payload: map[string]interface{}{"message": "created issue"}},
		{Timestamp: now.Add(-1 * time.Minute).Format(time.RFC3339), Source: "test", Type: "sling", Actor: "gastown/crew/joe", Visibility: "feed", Payload: map[string]interface{}{"bead": "gt-abc", "target": "polecat-1"}},
		{Timestamp: now.Format(time.RFC3339), Source: "test", Type: "done", Actor: "gastown/crew/joe", Visibility: "feed", Payload: map[string]interface{}{"bead": "gt-abc"}},
	})

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := PrintGtEvents(townRoot, PrintOptions{Limit: 10})

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("PrintGtEvents returned error: %v", err)
	}

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	// Should have 3 lines of output (oldest first)
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 output lines, got %d: %q", len(lines), output)
	}

	// First line should be the oldest event (create)
	if !strings.Contains(lines[0], "created issue") {
		t.Errorf("first line should contain 'created issue', got: %s", lines[0])
	}
	// Last line should be the newest event (done)
	if !strings.Contains(lines[2], "done") {
		t.Errorf("last line should contain 'done', got: %s", lines[2])
	}
}

func TestPrintGtEvents_LimitApplied(t *testing.T) {
	now := time.Now()
	var events []GtEvent
	for i := 0; i < 20; i++ {
		events = append(events, GtEvent{
			Timestamp:  now.Add(time.Duration(-20+i) * time.Minute).Format(time.RFC3339),
			Source:     "test",
			Type:       "create",
			Actor:      "test",
			Visibility: "feed",
			Payload:    map[string]interface{}{"message": "event"},
		})
	}
	townRoot := writeTestEvents(t, events)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := PrintGtEvents(townRoot, PrintOptions{Limit: 5})

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("PrintGtEvents returned error: %v", err)
	}

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 5 {
		t.Errorf("expected 5 output lines (limit), got %d", len(lines))
	}
}

func TestPrintGtEvents_SinceFilter(t *testing.T) {
	now := time.Now()
	townRoot := writeTestEvents(t, []GtEvent{
		{Timestamp: now.Add(-2 * time.Hour).Format(time.RFC3339), Source: "test", Type: "create", Actor: "old", Visibility: "feed", Payload: map[string]interface{}{"message": "old event"}},
		{Timestamp: now.Add(-30 * time.Second).Format(time.RFC3339), Source: "test", Type: "create", Actor: "new", Visibility: "feed", Payload: map[string]interface{}{"message": "new event"}},
	})

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := PrintGtEvents(townRoot, PrintOptions{Limit: 100, Since: "5m"})

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("PrintGtEvents returned error: %v", err)
	}

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 event after --since 5m, got %d: %q", len(lines), output)
	}
	if !strings.Contains(lines[0], "new event") {
		t.Errorf("expected recent event, got: %s", lines[0])
	}
}

func TestPrintGtEvents_TypeFilter(t *testing.T) {
	now := time.Now()
	townRoot := writeTestEvents(t, []GtEvent{
		{Timestamp: now.Add(-2 * time.Minute).Format(time.RFC3339), Source: "test", Type: "create", Actor: "a", Visibility: "feed", Payload: map[string]interface{}{"message": "created"}},
		{Timestamp: now.Add(-1 * time.Minute).Format(time.RFC3339), Source: "test", Type: "sling", Actor: "b", Visibility: "feed", Payload: map[string]interface{}{"bead": "gt-1", "target": "p1"}},
		{Timestamp: now.Format(time.RFC3339), Source: "test", Type: "create", Actor: "c", Visibility: "feed", Payload: map[string]interface{}{"message": "created again"}},
	})

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := PrintGtEvents(townRoot, PrintOptions{Limit: 100, Type: "create"})

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("PrintGtEvents returned error: %v", err)
	}

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 create events, got %d: %q", len(lines), output)
	}
}

func TestPrintGtEvents_NoEventsFile(t *testing.T) {
	dir := t.TempDir() // no .events.jsonl
	err := PrintGtEvents(dir, PrintOptions{Limit: 10})
	if err == nil {
		t.Fatal("expected error for missing events file")
	}
	if !strings.Contains(err.Error(), "no events file found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPrintGtEvents_VisibilityFiltering(t *testing.T) {
	now := time.Now()
	townRoot := writeTestEvents(t, []GtEvent{
		{Timestamp: now.Format(time.RFC3339), Source: "test", Type: "create", Actor: "a", Visibility: "feed", Payload: map[string]interface{}{"message": "visible"}},
		{Timestamp: now.Format(time.RFC3339), Source: "test", Type: "create", Actor: "b", Visibility: "internal", Payload: map[string]interface{}{"message": "hidden"}},
		{Timestamp: now.Format(time.RFC3339), Source: "test", Type: "create", Actor: "c", Visibility: "both", Payload: map[string]interface{}{"message": "also visible"}},
	})

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := PrintGtEvents(townRoot, PrintOptions{Limit: 100})

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("PrintGtEvents returned error: %v", err)
	}

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 visible events, got %d: %q", len(lines), output)
	}
}

func TestPrintGtEvents_InvalidSinceDuration(t *testing.T) {
	now := time.Now()
	townRoot := writeTestEvents(t, []GtEvent{
		{Timestamp: now.Format(time.RFC3339), Source: "test", Type: "create", Actor: "a", Visibility: "feed", Payload: map[string]interface{}{"message": "event"}},
	})

	err := PrintGtEvents(townRoot, PrintOptions{Limit: 10, Since: "notaduration"})
	if err == nil {
		t.Fatal("expected error for invalid --since duration")
	}
	if !strings.Contains(err.Error(), "invalid --since duration") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPrintGtEvents_FollowStreamsAppended(t *testing.T) {
	now := time.Now()
	dir := t.TempDir()
	eventsPath := filepath.Join(dir, ".events.jsonl")

	// Write initial event
	initial, _ := json.Marshal(GtEvent{
		Timestamp: now.Format(time.RFC3339), Source: "test", Type: "create",
		Actor: "a", Visibility: "feed", Payload: map[string]interface{}{"message": "initial"},
	})
	os.WriteFile(eventsPath, append(initial, '\n'), 0644)

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	var printErr error

	// Run PrintGtEvents with Follow in a goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		printErr = PrintGtEvents(dir, PrintOptions{Limit: 100, Follow: true, Ctx: ctx})
	}()

	// Wait for initial event to be printed, then append a second event
	time.Sleep(500 * time.Millisecond)

	appended, _ := json.Marshal(GtEvent{
		Timestamp: now.Add(1 * time.Second).Format(time.RFC3339), Source: "test", Type: "sling",
		Actor: "b", Visibility: "feed", Payload: map[string]interface{}{"bead": "gt-1", "target": "p1"},
	})
	f, _ := os.OpenFile(eventsPath, os.O_APPEND|os.O_WRONLY, 0644)
	f.Write(append(appended, '\n'))
	f.Close()

	// Wait for the tail loop to pick it up, then cancel
	time.Sleep(500 * time.Millisecond)
	cancel()
	wg.Wait()

	w.Close()
	os.Stdout = oldStdout

	buf := make([]byte, 8192)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if printErr != nil {
		t.Logf("PrintGtEvents returned: %v (expected for follow mode)", printErr)
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 output lines (initial + appended), got %d: %q", len(lines), output)
	}

	if !strings.Contains(lines[0], "initial") {
		t.Errorf("first line should contain 'initial', got: %s", lines[0])
	}
	// The appended event should have been picked up by the tail loop
	found := false
	for _, line := range lines[1:] {
		if strings.Contains(line, "slung") || strings.Contains(line, "work slung") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected appended event in output, got: %q", output)
	}
}

func TestPrintGtEvents_RigFilter(t *testing.T) {
	now := time.Now()
	townRoot := writeTestEvents(t, []GtEvent{
		{Timestamp: now.Add(-2 * time.Minute).Format(time.RFC3339), Source: "test", Type: "create", Actor: "greenplace/witness", Visibility: "feed", Payload: map[string]interface{}{"message": "greenplace event", "rig": "greenplace"}},
		{Timestamp: now.Add(-1 * time.Minute).Format(time.RFC3339), Source: "test", Type: "create", Actor: "bluecove/witness", Visibility: "feed", Payload: map[string]interface{}{"message": "bluecove event", "rig": "bluecove"}},
		{Timestamp: now.Format(time.RFC3339), Source: "test", Type: "sling", Actor: "greenplace/crew/joe", Visibility: "feed", Payload: map[string]interface{}{"bead": "gt-1", "target": "p1", "rig": "greenplace"}},
	})

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := PrintGtEvents(townRoot, PrintOptions{Limit: 100, Rig: "greenplace"})

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("PrintGtEvents returned error: %v", err)
	}

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 greenplace events, got %d: %q", len(lines), output)
	}
	for _, line := range lines {
		if strings.Contains(line, "bluecove") {
			t.Errorf("rig filter should exclude bluecove events, got: %s", line)
		}
	}
}

func TestMatchesFilters(t *testing.T) {
	now := time.Now()
	event := &Event{
		Time:    now,
		Type:    "create",
		Actor:   "gastown/witness",
		Target:  "gt-abc-123",
		Message: "created issue mol-42",
		Rig:     "greenplace",
	}

	tests := []struct {
		name      string
		sinceTime time.Time
		mol       string
		eventType string
		rig       string
		want      bool
	}{
		{"no filters", time.Time{}, "", "", "", true},
		{"since matches", now.Add(-1 * time.Minute), "", "", "", true},
		{"since excludes", now.Add(1 * time.Minute), "", "", "", false},
		{"mol matches target", time.Time{}, "gt-abc", "", "", true},
		{"mol matches message", time.Time{}, "mol-42", "", "", true},
		{"mol no match", time.Time{}, "nonexistent", "", "", false},
		{"type matches", time.Time{}, "", "create", "", true},
		{"type no match", time.Time{}, "", "delete", "", false},
		{"rig matches", time.Time{}, "", "", "greenplace", true},
		{"rig no match", time.Time{}, "", "", "otherrig", false},
		{"combined all match", now.Add(-1 * time.Minute), "gt-abc", "create", "", true},
		{"combined type mismatch", now.Add(-1 * time.Minute), "gt-abc", "delete", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := matchesFilters(event, tc.sinceTime, tc.mol, tc.eventType, tc.rig)
			if got != tc.want {
				t.Errorf("matchesFilters() = %v, want %v", got, tc.want)
			}
		})
	}
}

func tailTestEvent(t *testing.T, at time.Time, message string) string {
	t.Helper()
	b, err := json.Marshal(GtEvent{
		Timestamp: at.Format(time.RFC3339), Source: "test", Type: "create",
		Actor: "a", Visibility: "feed", Payload: map[string]interface{}{"message": message},
	})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	return string(b) + "\n"
}

func appendToFile(t *testing.T, path, data string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		t.Fatalf("open for append: %v", err)
	}
	defer f.Close()
	if _, err := f.WriteString(data); err != nil {
		t.Fatalf("append: %v", err)
	}
}

// replaceFile mimics gt krc prune: write a temp file, then rename it over path.
func replaceFile(t *testing.T, path, data string) {
	t.Helper()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(data), 0644); err != nil {
		t.Fatalf("write tmp: %v", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		t.Fatalf("rename: %v", err)
	}
}

func newTestTailer(t *testing.T, path string) (*eventTailer, *[]string) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	tail := newEventTailer(path, file)
	t.Cleanup(tail.close)
	// Like PrintGtEvents: read the file to the end, counting every line shown.
	sc := bufio.NewScanner(file)
	for sc.Scan() {
		tail.observeLine(sc.Text())
	}
	tail.resumeAtOffsetOf(file)
	got := &[]string{}
	return tail, got
}

func pollMessages(t *testing.T, tail *eventTailer, got *[]string) []string {
	t.Helper()
	*got = nil
	if err := tail.poll(func(e Event) { *got = append(*got, e.Message) }); err != nil {
		t.Fatalf("poll: %v", err)
	}
	return *got
}

func TestEventTailer_AppendReplaceAndTruncate(t *testing.T) {
	base := time.Now().Add(-time.Hour).Truncate(time.Second)
	path := filepath.Join(t.TempDir(), ".events.jsonl")
	appendToFile(t, path, tailTestEvent(t, base, "one")+tailTestEvent(t, base.Add(time.Second), "two"))

	tail, got := newTestTailer(t, path)

	if m := pollMessages(t, tail, got); len(m) != 0 {
		t.Fatalf("no new data: expected nothing, got %v", m)
	}

	appendToFile(t, path, tailTestEvent(t, base.Add(2*time.Second), "three"))
	if m := pollMessages(t, tail, got); len(m) != 1 || !strings.Contains(m[0], "three") {
		t.Fatalf("append: expected [three], got %v", m)
	}

	// Prune-style replace: retained history ("two", "three") plus an event that
	// landed in the new file before the next poll ("four").
	replaceFile(t, path,
		tailTestEvent(t, base.Add(time.Second), "two")+
			tailTestEvent(t, base.Add(2*time.Second), "three")+
			tailTestEvent(t, base.Add(3*time.Second), "four"))
	if m := pollMessages(t, tail, got); len(m) != 1 || !strings.Contains(m[0], "four") {
		t.Fatalf("replace: expected only [four] (no repeats), got %v", m)
	}

	appendToFile(t, path, tailTestEvent(t, base.Add(4*time.Second), "five"))
	if m := pollMessages(t, tail, got); len(m) != 1 || !strings.Contains(m[0], "five") {
		t.Fatalf("append after replace: expected [five], got %v", m)
	}

	// Truncate in place (copytruncate style) and write fresh events.
	if err := os.Truncate(path, 0); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	appendToFile(t, path, tailTestEvent(t, base.Add(5*time.Second), "six"))
	if m := pollMessages(t, tail, got); len(m) != 1 || !strings.Contains(m[0], "six") {
		t.Fatalf("truncate: expected [six], got %v", m)
	}
}

func TestEventTailer_DrainsOldFileBeforeSwitching(t *testing.T) {
	base := time.Now().Add(-time.Hour).Truncate(time.Second)
	path := filepath.Join(t.TempDir(), ".events.jsonl")
	appendToFile(t, path, tailTestEvent(t, base, "one"))

	tail, got := newTestTailer(t, path)

	// Appended to the old inode, then replaced before the tailer polled. The
	// replacement does not contain it (the prune read the file earlier).
	appendToFile(t, path, tailTestEvent(t, base.Add(time.Second), "late"))
	replaceFile(t, path, tailTestEvent(t, base, "one"))

	if m := pollMessages(t, tail, got); len(m) != 1 || !strings.Contains(m[0], "late") {
		t.Fatalf("expected [late] from the old inode, got %v", m)
	}
}

func TestEventTailer_ReassemblesSplitLine(t *testing.T) {
	base := time.Now().Add(-time.Hour).Truncate(time.Second)
	path := filepath.Join(t.TempDir(), ".events.jsonl")
	appendToFile(t, path, tailTestEvent(t, base, "one"))

	tail, got := newTestTailer(t, path)

	line := tailTestEvent(t, base.Add(time.Second), "split")
	appendToFile(t, path, line[:20])
	if m := pollMessages(t, tail, got); len(m) != 0 {
		t.Fatalf("half a line must not be emitted, got %v", m)
	}
	appendToFile(t, path, line[20:])
	if m := pollMessages(t, tail, got); len(m) != 1 || !strings.Contains(m[0], "split") {
		t.Fatalf("expected [split] once the line completes, got %v", m)
	}
}

// A prune can retain lines whose timestamp does not parse. parseGtEventLine
// stamps those with the read time, so a time-based dedup would reprint them
// after every replace.
func TestEventTailer_ReplaceDoesNotRepeatUnparseableTimestamp(t *testing.T) {
	base := time.Now().Add(-time.Hour).Truncate(time.Second)
	path := filepath.Join(t.TempDir(), ".events.jsonl")
	badTS := `{"ts":"not-a-time","source":"test","type":"create","actor":"a","visibility":"feed","payload":{"message":"bad-ts"}}` + "\n"
	appendToFile(t, path, badTS+tailTestEvent(t, base, "one"))

	tail, got := newTestTailer(t, path)

	replaceFile(t, path, badTS+tailTestEvent(t, base, "one")+tailTestEvent(t, base.Add(time.Second), "two"))
	if m := pollMessages(t, tail, got); len(m) != 1 || !strings.Contains(m[0], "two") {
		t.Fatalf("replace: expected only [two], got %v", m)
	}

	replaceFile(t, path, badTS+tailTestEvent(t, base, "one")+tailTestEvent(t, base.Add(time.Second), "two"))
	if m := pollMessages(t, tail, got); len(m) != 0 {
		t.Fatalf("second replace: expected nothing, got %v", m)
	}
}

// Timestamps have one-second resolution, so a new event can be byte-identical
// to an old one. It must still be shown, and the retained copy must not repeat.
func TestEventTailer_ReplaceKeepsNewEventIdenticalToOldOne(t *testing.T) {
	base := time.Now().Add(-time.Hour).Truncate(time.Second)
	path := filepath.Join(t.TempDir(), ".events.jsonl")
	same := tailTestEvent(t, base, "same")
	appendToFile(t, path, same)

	tail, got := newTestTailer(t, path)

	// Replace keeps the old line, and the same line was logged again before
	// the next poll.
	replaceFile(t, path, same+same)
	if m := pollMessages(t, tail, got); len(m) != 1 || !strings.Contains(m[0], "same") {
		t.Fatalf("expected the second copy shown once, got %v", m)
	}

	replaceFile(t, path, same+same)
	if m := pollMessages(t, tail, got); len(m) != 0 {
		t.Fatalf("expected no repeats, got %v", m)
	}
}

func TestPrintGtEvents_FollowSurvivesFileReplacement(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	dir := t.TempDir()
	eventsPath := filepath.Join(dir, ".events.jsonl")
	appendToFile(t, eventsPath, tailTestEvent(t, now, "initial"))

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = PrintGtEvents(dir, PrintOptions{Limit: 100, Follow: true, Ctx: ctx})
	}()

	time.Sleep(500 * time.Millisecond)
	replaceFile(t, eventsPath, tailTestEvent(t, now, "initial"))
	time.Sleep(500 * time.Millisecond)
	appendToFile(t, eventsPath, tailTestEvent(t, now.Add(time.Second), "after-replace"))
	time.Sleep(500 * time.Millisecond)
	cancel()
	wg.Wait()

	w.Close()
	os.Stdout = oldStdout
	buf := make([]byte, 8192)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if strings.Count(output, "initial") != 1 {
		t.Errorf("initial event should print exactly once, got output: %q", output)
	}
	if !strings.Contains(output, "after-replace") {
		t.Errorf("event appended after the file was replaced never streamed: %q", output)
	}
}
