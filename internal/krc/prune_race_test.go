package krc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/events"
)

// prefillEvents writes a mix of expired and fresh events so every prune has
// real scanning and rewriting to do, which widens the read-to-rename window.
func prefillEvents(t *testing.T, path string, expired, fresh int) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	old := time.Now().Add(-72 * time.Hour).UTC().Format(time.RFC3339)
	now := time.Now().UTC().Format(time.RFC3339)
	for i := 0; i < expired; i++ {
		fmt.Fprintf(w, `{"ts":%q,"source":"gt","type":"patrol_started","actor":"prefill","visibility":"feed"}`+"\n", old)
	}
	for i := 0; i < fresh; i++ {
		fmt.Fprintf(w, `{"ts":%q,"source":"gt","type":"merge_started","actor":"prefill","visibility":"feed"}`+"\n", now)
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("flush %s: %v", path, err)
	}
}

func seenAppendIDs(t *testing.T, path string) map[int]int {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	seen := map[int]int{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		var ev struct {
			Actor   string `json:"actor"`
			Payload struct {
				N int `json:"n"`
			} `json:"payload"`
		}
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil || ev.Actor != "appender" {
			continue
		}
		seen[ev.Payload.N]++
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan %s: %v", path, err)
	}
	return seen
}

// Events written through the real writer while prunes run must all survive:
// the writer and the pruner share one lock, so a prune cannot rename over a
// file that has an append in flight.
func TestPrune_DoesNotLoseConcurrentAppends(t *testing.T) {
	townRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(townRoot, "mayor"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(townRoot, "mayor", "town.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(townRoot)

	eventsPath := filepath.Join(townRoot, events.EventsFile)
	prefillEvents(t, eventsPath, 5000, 5000)

	// Keep appending for as long as the prunes run, so some appends land
	// between a prune's read and its rename however long each phase takes.
	const prunes = 10
	stop := make(chan struct{})
	appended := make(chan int, 1)
	go func() {
		n := 0
		for {
			select {
			case <-stop:
				appended <- n
				return
			default:
			}
			if err := events.LogFeed("merge_started", "appender", map[string]interface{}{"n": n}); err != nil {
				t.Errorf("append %d: %v", n, err)
				appended <- n
				return
			}
			n++
			time.Sleep(time.Millisecond)
		}
	}()

	pruner := NewPruner(townRoot, DefaultConfig())
	for i := 0; i < prunes; i++ {
		if _, err := pruner.Prune(); err != nil {
			close(stop)
			t.Fatalf("prune: %v", err)
		}
	}
	close(stop)
	total := <-appended

	seen := seenAppendIDs(t, eventsPath)
	var lost []int
	for i := 0; i < total; i++ {
		if seen[i] != 1 {
			lost = append(lost, i)
		}
	}
	if len(lost) > 0 {
		t.Fatalf("%d of %d appends lost across %d prunes, first ids: %v", len(lost), total, prunes, lost[:min(len(lost), 10)])
	}
}
