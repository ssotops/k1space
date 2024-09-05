package main

import (
  "strings"
	"sync"
	"time"
  "os/exec"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
)

const maxLogLines = 100

type scrollingLog struct {
	lines []string
	mu    sync.Mutex
}

func (sl *scrollingLog) add(line string) {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	sl.lines = append(sl.lines, line)
	if len(sl.lines) > maxLogLines {
		sl.lines = sl.lines[len(sl.lines)-maxLogLines:]
	}
}

func (sl *scrollingLog) getLastN(n int) []string {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	if len(sl.lines) <= n {
		return sl.lines
	}
	return sl.lines[len(sl.lines)-n:]
}

func (sl *scrollingLog) get() string {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	return strings.Join(sl.lines, "\n")
}

func startSpinner(message string) *spinner.Spinner {
	s := spinner.New(spinner.CharSets[9], 100*time.Millisecond)
	s.Suffix = " " + message
	s.Start()
	return s
}

func stopSpinner(s *spinner.Spinner, success bool) {
	s.Stop()
	if success {
		color.Green("✓ " + s.Suffix)
	} else {
		color.Red("✗ " + s.Suffix)
	}
}

func getRepoStatus(repoPath string) (string, error) {
    cmd := exec.Command("git", "-C", repoPath, "status", "--porcelain")
    output, err := cmd.Output()
    if err != nil {
        return "", err
    }
    
    if len(output) == 0 {
        return "Clean", nil
    }
    return "Has local changes", nil
}
