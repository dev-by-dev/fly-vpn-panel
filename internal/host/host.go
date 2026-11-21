// Package host reads host-level resource usage for the panel's main
// page: CPU, memory and the filesystem behind DATA_DIR.
package host

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Stats is the JSON body of GET /api/system.
type Stats struct {
	CPUUsedPercent float64 `json:"cpu_used_percent"`
	CPUCores       int     `json:"cpu_cores"`
	MemTotal       int64   `json:"mem_total"`
	MemUsed        int64   `json:"mem_used"`
	MemFree        int64   `json:"mem_free"`
	DiskTotal      int64   `json:"disk_total"`
	DiskFree       int64   `json:"disk_free"`
}

// Read samples host usage. CPU and memory come from /proc, so they stay
// zero on non-Linux hosts (local macOS development); disk works anywhere.
func Read(dataDir string) (Stats, error) {
	s := Stats{CPUCores: runtime.NumCPU()}
	if runtime.GOOS == "linux" {
		if pct, err := cpuPercent(); err == nil {
			s.CPUUsedPercent = pct
		}
		if total, avail, err := meminfo(); err == nil {
			s.MemTotal, s.MemFree = total, avail
			s.MemUsed = total - avail
		}
	}
	total, free, err := disk(dataDir)
	if err != nil {
		return s, err
	}
	s.DiskTotal, s.DiskFree = total, free
	return s, nil
}

// cpuTimes is the aggregate /proc/stat "cpu" line as tick counters.
type cpuTimes struct {
	idle, total uint64
}

// parseCPULine parses the aggregate "cpu" line; guest/guest_nice are
// skipped because the kernel already counts them in user/nice.
func parseCPULine(line string) (cpuTimes, error) {
	fields := strings.Fields(line)
	if len(fields) < 5 || fields[0] != "cpu" {
		return cpuTimes{}, fmt.Errorf("host: bad /proc/stat cpu line %q", line)
	}
	var c cpuTimes
	for i, f := range fields[1:] {
		if i >= 8 {
			break
		}
		v, err := strconv.ParseUint(f, 10, 64)
		if err != nil {
			return cpuTimes{}, fmt.Errorf("host: parse /proc/stat: %w", err)
		}
		c.total += v
		if i == 3 || i == 4 { // idle + iowait
			c.idle += v
		}
	}
	return c, nil
}

func readCPUTimes() (cpuTimes, error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return cpuTimes{}, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		return cpuTimes{}, fmt.Errorf("host: /proc/stat: %w", sc.Err())
	}
	return parseCPULine(sc.Text())
}

// cpuPercent measures utilization over a short interval: /proc/stat is
// cumulative, so one sample only gives the average since boot.
func cpuPercent() (float64, error) {
	a, err := readCPUTimes()
	if err != nil {
		return 0, err
	}
	time.Sleep(100 * time.Millisecond)
	b, err := readCPUTimes()
	if err != nil {
		return 0, err
	}
	total := b.total - a.total
	if total == 0 {
		return 0, nil
	}
	return 100 * float64(total-(b.idle-a.idle)) / float64(total), nil
}

// parseMeminfo returns MemTotal and the memory available for new work
// (MemAvailable, falling back to MemFree on very old kernels), in bytes.
func parseMeminfo(data string) (total, avail int64, err error) {
	var free int64
	for _, line := range strings.Split(data, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		v, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			total = v * 1024
		case "MemAvailable:":
			avail = v * 1024
		case "MemFree:":
			free = v * 1024
		}
	}
	if total == 0 {
		return 0, 0, fmt.Errorf("host: /proc/meminfo has no MemTotal")
	}
	if avail == 0 {
		avail = free
	}
	return total, avail, nil
}

func meminfo() (total, avail int64, err error) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0, err
	}
	return parseMeminfo(string(data))
}

func disk(dir string) (total, free int64, err error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return 0, 0, fmt.Errorf("host: statfs %s: %w", dir, err)
	}
	bs := int64(st.Bsize)
	return int64(st.Blocks) * bs, int64(st.Bavail) * bs, nil
}
