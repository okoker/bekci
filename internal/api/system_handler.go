package api

import (
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	probing "github.com/prometheus-community/pro-bing"
)

type netHealth struct {
	Status    string `json:"status"`
	LatencyMs int64  `json:"latency_ms"`
}

type diskHealth struct {
	TotalGB float64 `json:"total_gb"`
	FreeGB  float64 `json:"free_gb"`
}

type cpuHealth struct {
	Load1  float64 `json:"load1"`
	NumCPU int     `json:"num_cpu"`
}

func (s *Server) handleSystemHealth(w http.ResponseWriter, r *http.Request) {
	result := map[string]any{
		"net":  checkNet(),
		"disk": checkDisk(s.dbPath),
		"cpu":  checkCPU(),
	}
	writeJSON(w, http.StatusOK, result)
}

// checkNet pings 1.1.1.1 (Cloudflare DNS) with a single ICMP packet.
func checkNet() netHealth {
	pinger, err := probing.NewPinger("1.1.1.1")
	if err != nil {
		return netHealth{Status: "unreachable", LatencyMs: -1}
	}
	pinger.Count = 1
	pinger.Timeout = 3 * time.Second
	pinger.SetPrivileged(os.Getuid() == 0)

	if err := pinger.Run(); err != nil {
		return netHealth{Status: "unreachable", LatencyMs: -1}
	}
	stats := pinger.Statistics()
	if stats.PacketsRecv == 0 {
		return netHealth{Status: "unreachable", LatencyMs: -1}
	}
	return netHealth{Status: "ok", LatencyMs: stats.AvgRtt.Milliseconds()}
}

// checkDisk uses Statfs on the directory containing the DB file.
func checkDisk(dbPath string) diskHealth {
	path := dbPath
	if path == "" {
		path = "."
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		if err := syscall.Statfs(".", &stat); err != nil {
			return diskHealth{}
		}
	}
	totalGB := float64(stat.Blocks*uint64(stat.Bsize)) / (1 << 30)
	freeGB := float64(stat.Bavail*uint64(stat.Bsize)) / (1 << 30)
	return diskHealth{
		TotalGB: round2(totalGB),
		FreeGB:  round2(freeGB),
	}
}

// checkCPU reads load average and core count.
func checkCPU() cpuHealth {
	return cpuHealth{
		Load1:  readLoad1(),
		NumCPU: runtime.NumCPU(),
	}
}

func readLoad1() float64 {
	// Linux: /proc/loadavg
	if data, err := os.ReadFile("/proc/loadavg"); err == nil {
		parts := strings.Fields(string(data))
		if len(parts) >= 1 {
			if v, err := strconv.ParseFloat(parts[0], 64); err == nil {
				return round2(v)
			}
		}
	}
	// macOS: sysctl vm.loadavg → "{ 1.23 4.56 7.89 }"
	if runtime.GOOS == "darwin" {
		if out, err := exec.Command("sysctl", "-n", "vm.loadavg").Output(); err == nil {
			s := strings.Trim(string(out), "{ }\n\r\t")
			parts := strings.Fields(s)
			if len(parts) >= 1 {
				if v, err := strconv.ParseFloat(parts[0], 64); err == nil {
					return round2(v)
				}
			}
		}
	}
	return -1
}

func round2(f float64) float64 {
	return float64(int(f*100)) / 100
}
