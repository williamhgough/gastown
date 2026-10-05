package cmd

import (
	"os"
	"path/filepath"
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
