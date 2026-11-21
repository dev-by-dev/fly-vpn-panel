package host

import "testing"

func TestParseCPULine(t *testing.T) {
	// user nice system idle iowait irq softirq steal guest guest_nice
	c, err := parseCPULine("cpu  10 20 30 40 50 60 70 80 90 100")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if want := uint64(360); c.total != want { // 10..80, guest columns skipped
		t.Errorf("total = %d, want %d", c.total, want)
	}
	if want := uint64(90); c.idle != want { // idle + iowait
		t.Errorf("idle = %d, want %d", c.idle, want)
	}
}

func TestParseCPULineRejectsGarbage(t *testing.T) {
	if _, err := parseCPULine("cpu0 1 2 3 4 5"); err == nil {
		t.Error("per-core line accepted, want error")
	}
}

func TestParseMeminfo(t *testing.T) {
	const fixture = "MemTotal:       16384000 kB\nMemFree:         1234000 kB\nMemAvailable:    9876000 kB\n"
	total, avail, err := parseMeminfo(fixture)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if want := int64(16384000 * 1024); total != want {
		t.Errorf("total = %d, want %d", total, want)
	}
	if want := int64(9876000 * 1024); avail != want {
		t.Errorf("avail = %d, want %d", avail, want)
	}
}

func TestParseMeminfoFallsBackToMemFree(t *testing.T) {
	const fixture = "MemTotal:       1000 kB\nMemFree:         250 kB\n"
	_, avail, err := parseMeminfo(fixture)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if want := int64(250 * 1024); avail != want {
		t.Errorf("avail = %d, want %d", avail, want)
	}
}

func TestReadReportsDisk(t *testing.T) {
	s, err := Read(".")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if s.DiskTotal <= 0 || s.DiskFree <= 0 || s.DiskFree > s.DiskTotal {
		t.Errorf("disk total/free = %d/%d, want 0 < free <= total", s.DiskTotal, s.DiskFree)
	}
}
