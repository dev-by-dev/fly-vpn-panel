// Package monitoring reads panel-relevant logs and Caddy metrics.
package monitoring

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	maxLines     = 1000
	defaultLines = 200
)

// Entry is one log line shown in the panel.
type Entry struct {
	Time    time.Time `json:"time"`
	Source  string    `json:"source"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
}

// Runner runs an external command and returns its stdout. Tests replace it.
type Runner func(name string, args ...string) ([]byte, error)

// Reader reads logs for the monitoring page.
type Reader struct {
	DataDir string
	Run     Runner
}

// RunCommand is the real Runner used by the panel.
func RunCommand(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

// Read returns the last lines of the requested source: server (whole
// journal), caddy (caddy unit) or xray (DATA_DIR/xray-error.log).
func (r Reader) Read(source string, lines int) ([]Entry, error) {
	if lines <= 0 {
		lines = defaultLines
	}
	if lines > maxLines {
		lines = maxLines
	}
	switch source {
	case "server", "caddy":
		return r.readJournal(source, lines)
	case "xray":
		return r.readXray(lines)
	}
	return nil, fmt.Errorf("monitoring: invalid source %q", source)
}


type journalLine struct {
	Timestamp string          `json:"__REALTIME_TIMESTAMP"`
	Priority  string          `json:"PRIORITY"`
	Message   json.RawMessage `json:"MESSAGE"`
}

var priorities = map[string]string{
	"0": "emerg", "1": "alert", "2": "crit", "3": "err",
	"4": "warning", "5": "notice", "6": "info", "7": "debug",
}

func (r Reader) readJournal(source string, lines int) ([]Entry, error) {
	run := r.Run
	if run == nil {
		run = RunCommand
	}
	args := []string{"-o", "json", "-n", strconv.Itoa(lines), "--no-pager"}
	if source == "caddy" {
		args = append([]string{"-u", "caddy"}, args...)
	}
	out, err := run("journalctl", args...)
	if err != nil {
		return nil, fmt.Errorf("journalctl: %w", err)
	}
	entries := make([]Entry, 0, lines)
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var j journalLine
		if err := json.Unmarshal([]byte(line), &j); err != nil {
			continue
		}
		entries = append(entries, Entry{
			Time:    journalTime(j.Timestamp),
			Source:  source,
			Level:   journalLevel(j.Priority),
			Message: journalMessage(j.Message),
		})
	}
	return entries, nil
}

func journalTime(us string) time.Time {
	n, err := strconv.ParseInt(us, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.UnixMicro(n).UTC()
}

func journalLevel(p string) string {
	if l, ok := priorities[p]; ok {
		return l
	}
	return "info"
}

// journalMessage decodes MESSAGE, which journald emits as a string when it
// fits and as an array of bytes when the line is not valid UTF-8.
func journalMessage(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var b []byte
	if err := json.Unmarshal(raw, &b); err == nil {
		return string(b)
	}
	return ""
}

var xrayLevelRe = regexp.MustCompile(`(?i)\[(Warning|Error|Info)\]`)

func (r Reader) readXray(lines int) ([]Entry, error) {
	data, err := os.ReadFile(filepath.Join(r.DataDir, "xray-error.log"))
	if err != nil {
		if os.IsNotExist(err) {
			return []Entry{}, nil
		}
		return nil, fmt.Errorf("read xray log: %w", err)
	}
	all := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(all) == 1 && all[0] == "" {
		return []Entry{}, nil
	}
	if len(all) > lines {
		all = all[len(all)-lines:]
	}
	entries := make([]Entry, 0, len(all))
	for _, line := range all {
		entries = append(entries, Entry{Source: "xray", Level: xrayLevel(line), Message: line})
	}
	return entries, nil
}

func xrayLevel(line string) string {
	if m := xrayLevelRe.FindStringSubmatch(line); m != nil {
		return strings.ToLower(m[1])
	}
	return "info"
}
