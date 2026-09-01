package main

import (
	"bytes"
	"testing"
	"unicode/utf8"
)

func TestProgressLines(t *testing.T) {
	if got := progressLine(progressPending, 0, "Example", 0); got != "· Example — pending" {
		t.Fatalf("pending line = %q", got)
	}
	if got := progressLine(progressRunning, 0, "Example", 2); got != "⠋ Example — checking.." {
		t.Fatalf("running line = %q", got)
	}
	if got := progressLine(progressFinished, 0, "Example", 3); got != "✓ Example — finished..." {
		t.Fatalf("finished line = %q", got)
	}
}

func TestNonInteractiveProgressUsesStableLogLines(t *testing.T) {
	var output bytes.Buffer
	progress := newProjectProgress(&output, []string{"First", "Second"}, false)
	progress.Start()
	progress.MarkRunning(0)
	progress.MarkRequestFinished(0)
	progress.MarkRequestFinished(0)
	progress.MarkFinished(0)
	progress.MarkRunning(1)
	progress.MarkFinished(1)
	progress.Close()

	want := "Checking First…\nFinished First..\nChecking Second…\nFinished Second\n"
	if output.String() != want {
		t.Fatalf("progress output = %q, want %q", output.String(), want)
	}
}

func TestInteractiveProgressUsesRenderedRowCountAndTruncatesLongLines(t *testing.T) {
	var output bytes.Buffer
	progress := newProjectProgress(&output, []string{
		"A project with a name that would wrap in a narrow terminal",
		"Second",
		"Third",
	}, true)
	progress.width = 24
	progress.maxRows = 2

	progress.mu.Lock()
	progress.renderLocked()
	progress.states[0] = progressFinished
	progress.renderLocked()
	progress.mu.Unlock()

	if !bytes.Contains(output.Bytes(), []byte("\x1b[2A")) {
		t.Fatalf("redraw did not move by the two rendered rows: %q", output.String())
	}
	if bytes.Contains(output.Bytes(), []byte("\x1b[3A")) {
		t.Fatalf("redraw moved by the full project count: %q", output.String())
	}
	for _, line := range bytes.Split(output.Bytes(), []byte("\n")) {
		plain := bytes.TrimPrefix(line, []byte("\x1b[2A"))
		plain = bytes.TrimPrefix(plain, []byte("\r\x1b[2K"))
		if utf8.RuneCount(plain) > 23 {
			t.Fatalf("rendered line exceeds terminal width: %q", plain)
		}
	}
}

func TestProgressViewportAdvancesPastFinishedProjects(t *testing.T) {
	progress := newProjectProgress(&bytes.Buffer{}, []string{"First", "Second", "Third", "Fourth"}, true)
	progress.maxRows = 3
	progress.states[0] = progressFinished
	progress.states[1] = progressFinished

	lines := progress.progressLinesLocked()
	want := []string{"· Third — pending", "· Fourth — pending", "  Showing projects 3–4 of 4"}
	if len(lines) != len(want) {
		t.Fatalf("progress lines = %#v", lines)
	}
	for index := range want {
		if lines[index] != want[index] {
			t.Fatalf("progress line %d = %q, want %q", index, lines[index], want[index])
		}
	}
}
