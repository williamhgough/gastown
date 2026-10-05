package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBdShowSaysMissing(t *testing.T) {
	if !bdShowSaysMissing([]byte("Issue cf-x not found\nHint: this ID may have never existed"), "cf-x") {
		t.Error("bd's not-found answer should read as absent")
	}
	if bdShowSaysMissing([]byte("Issue cf-y not found"), "cf-x") {
		t.Error("another ID's not-found answer must not count")
	}
	if bdShowSaysMissing([]byte("bd: command not found"), "cf-x") {
		t.Error("a missing binary must not read as absent")
	}
	if bdShowSaysMissing([]byte("Error: failed to connect to dolt server"), "cf-x") {
		t.Error("a connection failure must not read as absent")
	}
}

func TestBeadCreateTargetDir(t *testing.T) {
	town := t.TempDir()
	if err := os.MkdirAll(filepath.Join(town, "cfx", ".beads"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(town, "mayor"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := beadCreateTargetDir(town, "cfx")
	if err != nil {
		t.Fatalf("explicit rig: %v", err)
	}
	if want := filepath.Join(town, "cfx"); got != want {
		t.Errorf("explicit rig dir = %q, want %q", got, want)
	}

	if _, err := beadCreateTargetDir(town, "mayor"); err == nil {
		t.Error("a directory with no .beads must not be accepted as an explicit rig")
	}
	if _, err := beadCreateTargetDir(town, "nope"); err == nil {
		t.Error("an unknown rig must be an error")
	}
}

// stubBdLoggingCreate installs a bd that answers the prefix and not-found
// lookups and appends every create call's arguments to the returned log file.
func stubBdLoggingCreate(t *testing.T) string {
	t.Helper()
	binDir := t.TempDir()
	logPath := filepath.Join(binDir, "create.log")
	script := `#!/bin/sh
case "${1:-}" in
  config) echo hq; exit 0 ;;
  show) echo "Issue $2 not found"; exit 1 ;;
  create) shift; printf '%s\n' "$*" >> "` + logPath + `"; exit 0 ;;
esac
exit 0
`
	if err := os.WriteFile(filepath.Join(binDir, "bd"), []byte(script), 0755); err != nil {
		t.Fatalf("write bd stub: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

func TestCreateReadableBeadPassesExternalRef(t *testing.T) {
	logPath := stubBdLoggingCreate(t)
	_, err := createReadableBead(t.TempDir(), readableBeadSpec{
		Title:       "request: reply to Ben",
		Type:        "task",
		Priority:    "2",
		ExternalRef: "slack:C123:1790.55",
	})
	if err != nil {
		t.Fatalf("createReadableBead: %v", err)
	}
	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("bd create was never called: %v", err)
	}
	if !strings.Contains(string(logged), "--external-ref=slack:C123:1790.55") {
		t.Errorf("bd create args missing the external ref: %s", logged)
	}
}

func TestCreateReadableBeadOmitsEmptyExternalRef(t *testing.T) {
	logPath := stubBdLoggingCreate(t)
	if _, err := createReadableBead(t.TempDir(), readableBeadSpec{Title: "plain task", Type: "task", Priority: "2"}); err != nil {
		t.Fatalf("createReadableBead: %v", err)
	}
	logged, _ := os.ReadFile(logPath)
	if strings.Contains(string(logged), "external-ref") {
		t.Errorf("an empty external ref must not be passed: %s", logged)
	}
}

func TestReadBeadDescriptionFromStdin(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteString("From: Ben\nbody text\n"); err != nil {
		t.Fatal(err)
	}
	w.Close()
	got, err := readBeadDescription("", "-", r)
	if err != nil {
		t.Fatalf("readBeadDescription: %v", err)
	}
	if got != "From: Ben\nbody text\n" {
		t.Errorf("stdin description = %q", got)
	}
	if _, err := readBeadDescription("inline", "-", r); err == nil {
		t.Error("--description with --description-file must be rejected")
	}
}
