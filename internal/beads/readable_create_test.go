package beads

import (
	"errors"
	"strconv"
	"testing"
	"time"
)

func TestReadableKindID(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		kind   string
		title  string
		want   string
	}{
		{"wisp keeps the -wisp- marker", "hq", "wisp", "Intake: 2 new item(s)", "hq-wisp-intake-2-new-item"},
		{"mail", "cf", "mail", "Re: gt dog done failure", "cf-mail-re-gt-dog-done-failure"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReadableKindID(tt.prefix, tt.kind, tt.title)
			if err != nil {
				t.Fatalf("ReadableKindID() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("ReadableKindID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReadableKindIDRejects(t *testing.T) {
	for name, args := range map[string][3]string{
		"empty prefix": {"", "wisp", "Intake"},
		"empty kind":   {"hq", "", "Intake"},
		"upper kind":   {"hq", "Wisp", "Intake"},
		"no words":     {"hq", "wisp", "the of -- !!"},
	} {
		t.Run(name, func(t *testing.T) {
			if got, err := ReadableKindID(args[0], args[1], args[2]); err == nil {
				t.Errorf("ReadableKindID() = %q, want an error", got)
			}
		})
	}
}

func TestIsDuplicateIDError(t *testing.T) {
	dup := errors.New("Error: hq-wisp-x already exists; use bd update, or bd import for upsert semantics")
	if !IsDuplicateIDError(dup) {
		t.Error("bd's duplicate-ID error should be recognised")
	}
	if IsDuplicateIDError(errors.New("failed to connect to dolt server")) {
		t.Error("a connection failure is not a duplicate")
	}
	if IsDuplicateIDError(nil) {
		t.Error("nil is not a duplicate")
	}
}

func TestCreateUnderReadableID(t *testing.T) {
	now := time.Unix(1790971354, 0)
	stamp := strconv.FormatInt(now.Unix(), 36)
	dup := errors.New("hq-wisp-x already exists")

	// failFirst makes create report a duplicate for the first n attempts.
	failFirst := func(n int, tried *[]string) func(string) error {
		return func(id string) error {
			*tried = append(*tried, id)
			if len(*tried) <= n {
				return dup
			}
			return nil
		}
	}

	tests := []struct {
		name      string
		dupes     int
		wantID    string
		wantTried []string
	}{
		{"free base", 0, "hq-wisp-x", []string{"hq-wisp-x"}},
		{"base taken", 1, "hq-wisp-x-2", []string{"hq-wisp-x", "hq-wisp-x-2"}},
		{"two taken", 2, "hq-wisp-x-3", []string{"hq-wisp-x", "hq-wisp-x-2", "hq-wisp-x-3"}},
		{"three taken uses a time token", 3, "hq-wisp-x-" + stamp, []string{"hq-wisp-x", "hq-wisp-x-2", "hq-wisp-x-3", "hq-wisp-x-" + stamp}},
		{"everything taken falls back to bd's own ID", 4, "", []string{"hq-wisp-x", "hq-wisp-x-2", "hq-wisp-x-3", "hq-wisp-x-" + stamp, ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tried []string
			got, err := CreateUnderReadableID("hq-wisp-x", now, failFirst(tt.dupes, &tried))
			if err != nil {
				t.Fatalf("CreateUnderReadableID() error = %v", err)
			}
			if got != tt.wantID {
				t.Errorf("CreateUnderReadableID() = %q, want %q", got, tt.wantID)
			}
			if len(tried) != len(tt.wantTried) {
				t.Fatalf("tried %v, want %v", tried, tt.wantTried)
			}
			for i := range tried {
				if tried[i] != tt.wantTried[i] {
					t.Errorf("attempt %d = %q, want %q", i, tried[i], tt.wantTried[i])
				}
			}
		})
	}
}

func TestCreateUnderReadableIDStopsOnOtherErrors(t *testing.T) {
	boom := errors.New("failed to connect to dolt server")
	attempts := 0
	_, err := CreateUnderReadableID("hq-wisp-x", time.Unix(0, 0), func(string) error {
		attempts++
		return boom
	})
	if !errors.Is(err, boom) {
		t.Errorf("error = %v, want it to wrap %v", err, boom)
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1: only a duplicate ID may trigger a retry", attempts)
	}
}
