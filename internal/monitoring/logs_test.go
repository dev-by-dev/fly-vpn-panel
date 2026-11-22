package monitoring

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func journalFixture() []byte {
	return []byte(`{"__REALTIME_TIMESTAMP":"1763800000000000","PRIORITY":"3","MESSAGE":"boom","SYSLOG_IDENTIFIER":"caddy","_SYSTEMD_UNIT":"caddy.service"}
{"__REALTIME_TIMESTAMP":"1763800001000000","PRIORITY":"6","MESSAGE":[104,105],"SYSLOG_IDENTIFIER":"caddy"}
not json
`)
}

func TestReadJournalJSON(t *testing.T) {
	var gotArgs []string
	r := Reader{Run: func(name string, args ...string) ([]byte, error) {
		gotArgs = append([]string{name}, args...)
		return journalFixture(), nil
	}}
	got, err := r.Read("caddy", 50)
	if err != nil {
		t.Fatalf("Read(caddy): %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("entries = %d, want 2 (bad line skipped)", len(got))
	}
	if got[0].Level != "err" || got[0].Message != "boom" || got[0].Source != "caddy" {
		t.Errorf("entry 0 = %+v, want level err message boom source caddy", got[0])
	}
	if got[0].Time.Unix() != 1763800000 {
		t.Errorf("entry 0 time = %v, want 1763800000", got[0].Time.Unix())
	}
	if got[1].Level != "info" || got[1].Message != "hi" {
		t.Errorf("entry 1 = %+v, want level info message hi (byte array MESSAGE)", got[1])
	}
	joined := strings.Join(gotArgs, " ")
	for _, want := range []string{"journalctl", "-u caddy", "-n 50", "--no-pager"} {
		if !strings.Contains(joined, want) {
			t.Errorf("journalctl args %q missing %q", joined, want)
		}
	}
}

func TestReadXrayFile(t *testing.T) {
	dir := t.TempDir()
	body := "[Warning] a\n[Error] b\nplain\n"
	if err := os.WriteFile(filepath.Join(dir, "xray-error.log"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Reader{DataDir: dir}.Read("xray", 2)
	if err != nil {
		t.Fatalf("Read(xray): %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("entries = %d, want last 2 lines", len(got))
	}
	if got[0].Level != "error" || got[0].Message != "[Error] b" || got[0].Source != "xray" {
		t.Errorf("entry 0 = %+v", got[0])
	}
	if got[1].Level != "info" || got[1].Message != "plain" {
		t.Errorf("entry 1 = %+v, want level info (no prefix)", got[1])
	}
}

func TestReadXrayMissingFile(t *testing.T) {
	got, err := Reader{DataDir: t.TempDir()}.Read("xray", 10)
	if err != nil {
		t.Fatalf("Read(xray) missing file: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("entries = %d, want 0", len(got))
	}
}

func TestReadClampsLines(t *testing.T) {
	for _, tc := range []struct {
		in   int
		want string
	}{{0, "-n 200"}, {5000, "-n 1000"}} {
		var args []string
		r := Reader{Run: func(name string, a ...string) ([]byte, error) {
			args = a
			return nil, nil
		}}
		if _, err := r.Read("server", tc.in); err != nil {
			t.Fatalf("Read(server, %d): %v", tc.in, err)
		}
		if !strings.Contains(strings.Join(args, " "), tc.want) {
			t.Errorf("lines %d → args %q, want %q", tc.in, strings.Join(args, " "), tc.want)
		}
	}
}

func TestRunCommandTimeout(t *testing.T) {
	old := commandTimeout
	commandTimeout = 50 * time.Millisecond
	defer func() { commandTimeout = old }()
	if _, err := RunCommand("sleep", "1"); err == nil {
		t.Fatal("RunCommand(sleep 1) = nil error, want timeout")
	}
}

func TestReadInvalidSource(t *testing.T) {
	if _, err := (Reader{}).Read("nope", 10); err == nil {
		t.Fatal("Read(nope) = nil error, want error")
	}
}

func TestReadJournalError(t *testing.T) {
	r := Reader{Run: func(string, ...string) ([]byte, error) { return nil, os.ErrPermission }}
	if _, err := r.Read("server", 10); err == nil || !strings.Contains(err.Error(), "journalctl") {
		t.Fatalf("Read(server) error = %v, want wrapped journalctl error", err)
	}
}

func TestClearXray(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "xray-error.log")
	if err := os.WriteFile(path, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := Reader{DataDir: dir}
	if err := r.ClearXray(); err != nil {
		t.Fatalf("ClearXray(): %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 0 {
		t.Fatalf("file size after clear = %d, want 0", len(data))
	}
}

func TestClearXrayMissingFile(t *testing.T) {
	if err := (Reader{DataDir: t.TempDir()}).ClearXray(); err != nil {
		t.Fatalf("ClearXray(missing) = %v, want nil", err)
	}
}
