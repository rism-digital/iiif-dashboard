package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/term"
)

type progressState int

const (
	progressPending progressState = iota
	progressRunning
	progressFinished
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type projectProgress struct {
	out          io.Writer
	interactive  bool
	names        []string
	states       []progressState
	requests     []int
	frame        int
	rendered     bool
	renderedRows int
	width        int
	maxRows      int
	stop         chan struct{}
	done         chan struct{}
	mu           sync.Mutex
}

func newProjectProgress(out io.Writer, names []string, interactive bool) *projectProgress {
	return &projectProgress{
		out:         out,
		interactive: interactive,
		names:       names,
		states:      make([]progressState, len(names)),
		requests:    make([]int, len(names)),
		stop:        make(chan struct{}),
		done:        make(chan struct{}),
	}
}

func (p *projectProgress) Start() {
	if !p.interactive || len(p.names) == 0 {
		return
	}
	p.mu.Lock()
	p.renderLocked()
	p.mu.Unlock()
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		defer close(p.done)
		for {
			select {
			case <-ticker.C:
				p.mu.Lock()
				p.frame = (p.frame + 1) % len(spinnerFrames)
				p.renderLocked()
				p.mu.Unlock()
			case <-p.stop:
				return
			}
		}
	}()
}

func (p *projectProgress) MarkRunning(index int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.states[index] = progressRunning
	if p.interactive {
		p.renderLocked()
	} else {
		fmt.Fprintf(p.out, "Checking %s…\n", p.names[index])
	}
}

func (p *projectProgress) MarkFinished(index int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.states[index] = progressFinished
	if p.interactive {
		p.renderLocked()
	} else {
		fmt.Fprintf(p.out, "Finished %s%s\n", p.names[index], requestDots(p.requests[index]))
	}
}

func (p *projectProgress) MarkRequestFinished(index int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.requests[index]++
	if p.interactive {
		p.renderLocked()
	}
}

func (p *projectProgress) Close() {
	if !p.interactive || len(p.names) == 0 {
		return
	}
	close(p.stop)
	<-p.done
}

func (p *projectProgress) renderLocked() {
	lines := p.progressLinesLocked()
	if p.rendered {
		fmt.Fprintf(p.out, "\x1b[%dA", p.renderedRows)
	}
	for _, line := range lines {
		fmt.Fprintf(p.out, "\r\x1b[2K%s\n", truncateProgressLine(line, p.width))
	}
	p.rendered = true
	p.renderedRows = len(lines)
}

func (p *projectProgress) progressLinesLocked() []string {
	rowLimit := p.maxRows
	if rowLimit <= 0 || rowLimit >= len(p.names) {
		lines := make([]string, len(p.names))
		for index, name := range p.names {
			lines[index] = progressLine(p.states[index], p.frame, name, p.requests[index])
		}
		return lines
	}
	if rowLimit == 1 {
		index := 0
		for index+1 < len(p.states) && p.states[index] == progressFinished {
			index++
		}
		return []string{progressLine(p.states[index], p.frame, p.names[index], p.requests[index])}
	}

	projectRows := rowLimit - 1
	start := 0
	for start < len(p.states) && p.states[start] == progressFinished {
		start++
	}
	if start+projectRows > len(p.names) {
		start = len(p.names) - projectRows
	}
	end := start + projectRows
	lines := make([]string, 0, rowLimit)
	for index := start; index < end; index++ {
		lines = append(lines, progressLine(p.states[index], p.frame, p.names[index], p.requests[index]))
	}
	lines = append(lines, fmt.Sprintf("  Showing projects %d–%d of %d", start+1, end, len(p.names)))
	return lines
}

func truncateProgressLine(line string, terminalWidth int) string {
	maximum := terminalWidth - 1
	if maximum < 2 || utf8.RuneCountInString(line) <= maximum {
		return line
	}
	runes := []rune(line)
	return string(runes[:maximum-1]) + "…"
}

func progressLine(state progressState, frame int, name string, requests int) string {
	dots := requestDots(requests)
	switch state {
	case progressRunning:
		return fmt.Sprintf("%s %s — checking%s", spinnerFrames[frame%len(spinnerFrames)], name, dots)
	case progressFinished:
		return fmt.Sprintf("✓ %s — finished%s", name, dots)
	default:
		return fmt.Sprintf("· %s — pending%s", name, dots)
	}
}

func requestDots(count int) string {
	return strings.Repeat(".", count)
}

func supportsInteractiveProgress(output *os.File) bool {
	if os.Getenv("CI") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	info, err := output.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func configureTerminalProgress(progress *projectProgress, output *os.File) {
	if !progress.interactive {
		return
	}
	progress.width = 80
	progress.maxRows = 20
	width, height, err := term.GetSize(int(output.Fd()))
	if err != nil {
		return
	}
	progress.width = width
	// Leave one terminal row beneath the progress area so writing the final
	// newline cannot scroll the viewport and invalidate cursor positioning.
	progress.maxRows = max(1, height-1)
}
