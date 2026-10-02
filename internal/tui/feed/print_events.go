package feed

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// PrintOptions controls filtering and behavior for PrintGtEvents.
type PrintOptions struct {
	Limit  int
	Follow bool
	Since  string // duration string like "5m", "1h"
	Mol    string // molecule/issue ID prefix filter
	Type   string // event type filter
	Rig    string // rig name filter (matches event's Rig field)
	Ctx    context.Context // optional: controls follow-mode lifecycle; nil uses signal.NotifyContext
}

// PrintGtEvents reads .events.jsonl and prints events to stdout.
// When opts.Follow is true, it tails the file for new events after printing
// the initial batch, polling every 200ms. Canceled via opts.Ctx or SIGINT.
func PrintGtEvents(townRoot string, opts PrintOptions) error {
	eventsPath := filepath.Join(townRoot, ".events.jsonl")
	file, err := os.Open(eventsPath)
	if err != nil {
		return fmt.Errorf("no events file found at %s: %w", eventsPath, err)
	}
	defer file.Close()

	// Parse --since into a cutoff time
	var sinceTime time.Time
	if opts.Since != "" {
		dur, err := time.ParseDuration(opts.Since)
		if err != nil {
			return fmt.Errorf("invalid --since duration %q: %w", opts.Since, err)
		}
		sinceTime = time.Now().Add(-dur)
	}

	tail := newEventTailer(eventsPath, file)

	var events []Event
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if opts.Follow {
			tail.observeLine(line)
		}
		if event := parseGtEventLine(line); event != nil {
			if matchesFilters(event, sinceTime, opts.Mol, opts.Type, opts.Rig) {
				events = append(events, *event)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading events: %w", err)
	}

	// Sort by time descending (most recent first)
	sort.Slice(events, func(i, j int) bool {
		return events[i].Time.After(events[j].Time)
	})

	// Apply limit
	if opts.Limit > 0 && len(events) > opts.Limit {
		events = events[:opts.Limit]
	}

	// Reverse to show oldest first (chronological)
	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}

	if len(events) == 0 && !opts.Follow {
		fmt.Println("No events found in .events.jsonl")
		return nil
	}

	for _, event := range events {
		printEvent(event)
	}

	if !opts.Follow {
		return nil
	}

	// Tail mode: poll the file every tick. The tailer survives the file being
	// replaced (gt krc prune rewrites it via tmp file + rename) or truncated.
	ctx := opts.Ctx
	if ctx == nil {
		var stop context.CancelFunc
		ctx, stop = signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
	}

	// The initial scan ran to EOF; the tailer tracks its own offset from there.
	tail.resumeAtOffsetOf(file)
	defer tail.close()

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			err := tail.poll(func(event Event) {
				if matchesFilters(&event, sinceTime, opts.Mol, opts.Type, opts.Rig) {
					printEvent(event)
				}
			})
			if err != nil {
				return fmt.Errorf("following events: %w", err)
			}
		}
	}
}

// eventTailer follows an events file by path. It reads whatever was appended
// since the last poll, reassembles lines split across polls, and re-opens the
// path when the file has been replaced or truncated underneath it.
//
// Following by file descriptor alone is not enough: gt krc prune (run hourly
// by the daemon) writes a temp file and renames it over .events.jsonl, which
// leaves an fd-based tail reading the unlinked old inode forever.
type eventTailer struct {
	path    string
	file    *os.File
	info    os.FileInfo
	offset  int64
	partial []byte

	// seen counts every raw line of the current file by content, whether or
	// not it parsed into a feed event. When the file is replaced, a line of
	// the new file that is still counted here was shown before and is skipped.
	// A prune only drops lines, so the survivors match the old ones one for
	// one, even when two lines are byte-identical. Matching on the raw line
	// keeps this independent of timestamps, which are only second-resolution
	// and fall back to the read time when they do not parse.
	seen map[string]int
}

func newEventTailer(path string, file *os.File) *eventTailer {
	t := &eventTailer{path: path, file: file, seen: map[string]int{}}
	t.info, _ = file.Stat()
	return t
}

// observeLine records a raw line that was already shown to the reader.
func (t *eventTailer) observeLine(line string) {
	if line == "" {
		return
	}
	t.seen[line]++
}

// resumeAtOffsetOf starts tailing from the file's current position.
func (t *eventTailer) resumeAtOffsetOf(file *os.File) {
	offset, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		offset = 0
	}
	t.offset = offset
	t.partial = nil
}

func (t *eventTailer) close() {
	_ = t.file.Close()
}

// poll emits events appended since the previous poll. When the path now refers
// to a different file, or the file shrank, it re-reads from the start and
// suppresses lines that were already emitted.
func (t *eventTailer) poll(emit func(Event)) error {
	// Drain the open file first so nothing appended before a rename is lost.
	if err := t.drain(emit, nil); err != nil {
		return err
	}

	current, err := os.Stat(t.path)
	if os.IsNotExist(err) {
		return nil // mid-replace; the next tick sees the new file
	}
	if err != nil {
		return fmt.Errorf("stat %s: %w", t.path, err)
	}

	replaced := t.info == nil || !os.SameFile(t.info, current)
	truncated := !replaced && current.Size() < t.offset
	if !replaced && !truncated {
		return nil
	}

	if replaced {
		next, err := os.Open(t.path)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return fmt.Errorf("reopen %s: %w", t.path, err)
		}
		_ = t.file.Close()
		t.file = next
		t.info, _ = next.Stat()
	} else if _, err := t.file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind %s: %w", t.path, err)
	}
	t.offset = 0
	t.partial = nil

	// Count the new file's lines afresh, so seen stays bounded by the size of
	// the current file. Lines the old file already showed are skipped once each.
	shown := t.seen
	t.seen = map[string]int{}
	return t.drain(emit, shown)
}

// drain reads to the end of the open file and emits each complete line.
// shown, when non-nil, holds lines from before a replace; each match is
// skipped and used up.
func (t *eventTailer) drain(emit func(Event), shown map[string]int) error {
	data, err := io.ReadAll(t.file)
	if err != nil {
		return fmt.Errorf("reading events: %w", err)
	}
	t.offset += int64(len(data))
	t.partial = append(t.partial, data...)

	for {
		i := bytes.IndexByte(t.partial, '\n')
		if i < 0 {
			return nil
		}
		line := string(t.partial[:i])
		t.partial = t.partial[i+1:]
		if line == "" {
			continue
		}

		t.seen[line]++
		if shown[line] > 0 {
			shown[line]--
			continue
		}

		if event := parseGtEventLine(line); event != nil {
			emit(*event)
		}
	}
}

// matchesFilters checks whether an event passes the --since, --mol, --type, and --rig filters.
func matchesFilters(event *Event, sinceTime time.Time, mol, eventType, rig string) bool {
	if !sinceTime.IsZero() && event.Time.Before(sinceTime) {
		return false
	}
	if mol != "" && !strings.Contains(event.Target, mol) && !strings.Contains(event.Message, mol) {
		return false
	}
	if eventType != "" && event.Type != eventType {
		return false
	}
	if rig != "" && event.Rig != rig {
		return false
	}
	return true
}

// printEvent formats and prints a single event line.
func printEvent(event Event) {
	symbol := typeSymbol(event.Type)
	ts := event.Time.Local().Format("15:04:05")
	actor := event.Actor
	if actor == "" {
		actor = "system"
	}
	fmt.Printf("[%s] %s %-25s %s\n", ts, symbol, actor, event.Message)
}

func typeSymbol(eventType string) string {
	switch eventType {
	case "patrol_started":
		return "\U0001F989" // owl
	case "patrol_complete":
		return "\U0001F989" // owl
	case "polecat_nudged":
		return "\u26A1" // lightning
	case "sling":
		return "\U0001F3AF" // target
	case "handoff":
		return "\U0001F91D" // handshake
	case "done":
		return "\u2713" // checkmark
	case "merged":
		return "\u2713"
	case "merge_failed":
		return "\u2717" // x
	case "create":
		return "+"
	case "complete":
		return "\u2713"
	case "fail":
		return "\u2717"
	case "delete":
		return "\u2298" // circled minus
	default:
		return "\u2192" // arrow
	}
}
