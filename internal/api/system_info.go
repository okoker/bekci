package api

import (
	"bufio"
	"context"
	"math"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"
)

// System Health tab (Settings → System Health). Admin-only: unlike the public
// /api/system/health (SOC dots), this exposes host details useful for recon.

type systemInfo struct {
	Disk      diskInfo        `json:"disk"`
	RAM       ramInfo         `json:"ram"`
	CPU       cpuInfo         `json:"cpu"`
	Uptime    uptimeInfo      `json:"uptime"`
	ServerIP  string          `json:"server_ip"`
	Version   string          `json:"version"`
	Build     buildInfo       `json:"build"`
	GoVersion string          `json:"go_version"`
	Database  databaseInfo    `json:"database"`
	Email     emailInfo       `json:"email"`
	Updates   updatesInfo     `json:"updates"`
	Backup    backupInfo      `json:"backup"`
	Log       logInfo         `json:"log"`
	Scheduler schedulerHealth `json:"scheduler"`
}

type diskInfo struct {
	TotalGB float64 `json:"total_gb"`
	FreeGB  float64 `json:"free_gb"`
	UsedPct float64 `json:"used_pct"`
}

type ramInfo struct {
	TotalGB float64 `json:"total_gb"` // 0 when unavailable
	UsedGB  float64 `json:"used_gb"`
	UsedPct float64 `json:"used_pct"`
}

type cpuInfo struct {
	Load1  float64 `json:"load1"` // -1 when unavailable
	Load5  float64 `json:"load5"`
	Load15 float64 `json:"load15"`
	NumCPU int     `json:"num_cpu"`
}

type uptimeInfo struct {
	SystemSec  int64 `json:"system_sec"` // -1 when unavailable
	ProcessSec int64 `json:"process_sec"`
}

type buildInfo struct {
	Commit   string `json:"commit"`             // short VCS revision, "" if not stamped
	Time     string `json:"time"`               // RFC3339 commit time, "" if not stamped
	Modified bool   `json:"modified,omitempty"` // built from a dirty tree
}

type databaseInfo struct {
	Status        string `json:"status"` // ok | error
	SizeBytes     int64  `json:"size_bytes"`
	WALBytes      int64  `json:"wal_bytes"`
	SchemaVersion int    `json:"schema_version"`
}

type emailInfo struct {
	Configured bool   `json:"configured"`
	Provider   string `json:"provider"`
}

type updatesInfo struct {
	Status    string `json:"status"` // up_to_date | updates | security | unavailable
	Total     int    `json:"total"`
	Security  int    `json:"security"`
	CheckedAt string `json:"checked_at,omitempty"`
}

type backupInfo struct {
	Count     int    `json:"count"`
	LastAt    string `json:"last_at,omitempty"`
	LastBytes int64  `json:"last_bytes"`
	Encrypted bool   `json:"encrypted"`
	Schedule  string `json:"schedule"` // auto_backup_schedule: off | weekly | 10days | monthly
	Time      string `json:"time"`     // auto_backup_time (HH:MM)
}

type logInfo struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"` // -1 when unknown
}

// SetLogPath tells the server where the app log lives (shown on System Health).
func (s *Server) SetLogPath(path string) {
	s.logPath = path
}

func (s *Server) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
	info := systemInfo{
		Disk:      readDiskInfo(s.dbPath),
		RAM:       readRAMInfo(),
		CPU:       readCPUInfo(),
		Uptime:    uptimeInfo{SystemSec: readSystemUptime(), ProcessSec: int64(time.Since(s.startedAt).Seconds())},
		ServerIP:  primaryIP(),
		Version:   s.version,
		Build:     readBuildInfo(),
		GoVersion: runtime.Version(),
		Database:  s.readDatabaseInfo(),
		Email:     s.readEmailInfo(),
		Updates:   cachedAptUpdates(r.Context()),
		Backup:    s.readBackupInfo(),
		Log:       readLogInfo(s.logPath),
	}
	if s.scheduler != nil {
		info.Scheduler = s.checkScheduler()
	} else {
		info.Scheduler = schedulerHealth{Status: "unavailable"}
	}
	writeJSON(w, http.StatusOK, info)
}

func readDiskInfo(dbPath string) diskInfo {
	d := checkDisk(dbPath)
	info := diskInfo{TotalGB: d.TotalGB, FreeGB: d.FreeGB}
	if d.TotalGB > 0 {
		info.UsedPct = round1((d.TotalGB - d.FreeGB) / d.TotalGB * 100)
	}
	return info
}

func readRAMInfo() ramInfo {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return ramInfo{}
	}
	total, avail, ok := parseMeminfo(string(data))
	if !ok || total == 0 {
		return ramInfo{}
	}
	used := total - avail
	return ramInfo{
		TotalGB: round2(float64(total) / (1 << 30)),
		UsedGB:  round2(float64(used) / (1 << 30)),
		UsedPct: round1(float64(used) / float64(total) * 100),
	}
}

// parseMeminfo returns MemTotal and MemAvailable in bytes.
func parseMeminfo(data string) (total, avail uint64, ok bool) {
	var haveTotal, haveAvail bool
	sc := bufio.NewScanner(strings.NewReader(data))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 2 {
			continue
		}
		v, err := strconv.ParseUint(f[1], 10, 64)
		if err != nil {
			continue
		}
		switch f[0] {
		case "MemTotal:":
			total, haveTotal = v*1024, true
		case "MemAvailable:":
			avail, haveAvail = v*1024, true
		}
	}
	return total, avail, haveTotal && haveAvail
}

func readCPUInfo() cpuInfo {
	info := cpuInfo{Load1: -1, Load5: -1, Load15: -1, NumCPU: runtime.NumCPU()}
	if data, err := os.ReadFile("/proc/loadavg"); err == nil {
		if l1, l5, l15, ok := parseLoadAvg(string(data)); ok {
			info.Load1, info.Load5, info.Load15 = l1, l5, l15
		}
	}
	return info
}

func parseLoadAvg(data string) (l1, l5, l15 float64, ok bool) {
	f := strings.Fields(data)
	if len(f) < 3 {
		return 0, 0, 0, false
	}
	var vals [3]float64
	for i := 0; i < 3; i++ {
		v, err := strconv.ParseFloat(f[i], 64)
		if err != nil {
			return 0, 0, 0, false
		}
		vals[i] = round2(v)
	}
	return vals[0], vals[1], vals[2], true
}

func readSystemUptime() int64 {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return -1
	}
	sec, ok := parseProcUptime(string(data))
	if !ok {
		return -1
	}
	return sec
}

func parseProcUptime(data string) (int64, bool) {
	f := strings.Fields(data)
	if len(f) < 1 {
		return 0, false
	}
	v, err := strconv.ParseFloat(f[0], 64)
	if err != nil {
		return 0, false
	}
	return int64(v), true
}

// primaryIP returns the source address the host would use for outbound
// traffic. Dialing UDP sends no packets; it only selects a route.
func primaryIP() string {
	conn, err := net.Dial("udp", "1.1.1.1:53")
	if err == nil {
		defer conn.Close()
		if a, ok := conn.LocalAddr().(*net.UDPAddr); ok && !a.IP.IsLoopback() {
			return a.IP.String()
		}
	}
	// No default route: fall back to the first non-loopback IPv4.
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok && !ipn.IP.IsLoopback() && ipn.IP.To4() != nil {
			return ipn.IP.String()
		}
	}
	return ""
}

func readBuildInfo() buildInfo {
	var b buildInfo
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return b
	}
	for _, kv := range bi.Settings {
		switch kv.Key {
		case "vcs.revision":
			b.Commit = kv.Value
			if len(b.Commit) > 7 {
				b.Commit = b.Commit[:7]
			}
		case "vcs.time":
			b.Time = kv.Value
		case "vcs.modified":
			b.Modified = kv.Value == "true"
		}
	}
	return b
}

func (s *Server) readDatabaseInfo() databaseInfo {
	info := databaseInfo{Status: "ok"}
	v, err := s.store.SchemaVersion()
	if err != nil {
		info.Status = "error"
	}
	info.SchemaVersion = v
	if s.dbPath != "" {
		if fi, err := os.Stat(s.dbPath); err == nil {
			info.SizeBytes = fi.Size()
		}
		if fi, err := os.Stat(s.dbPath + "-wal"); err == nil {
			info.WALBytes = fi.Size()
		}
	}
	return info
}

// readEmailInfo mirrors the alerter's own "configured" rules (alerter.SendTestEmail).
func (s *Server) readEmailInfo() emailInfo {
	get := func(k string) string { v, _ := s.store.GetSetting(k); return v }
	from := get("alert_from_email")
	if get("email_provider") == "ms365" {
		return emailInfo{
			Provider:   "Microsoft 365",
			Configured: from != "" && get("smtp_host") != "" && get("smtp_username") != "" && get("smtp_password") != "",
		}
	}
	return emailInfo{Provider: "Resend", Configured: from != "" && get("resend_api_key") != ""}
}

func (s *Server) readBackupInfo() backupInfo {
	info := backupInfo{}
	info.Schedule, _ = s.store.GetSetting("auto_backup_schedule")
	if info.Schedule == "" {
		info.Schedule = "off"
	}
	info.Time, _ = s.store.GetSetting("auto_backup_time")
	if info.Time == "" {
		info.Time = "03:00"
	}
	entries := loadBackupIndex(s.backupDir)
	info.Count = len(entries)
	for _, e := range entries {
		if e.CreatedAt > info.LastAt { // RFC3339 UTC sorts lexically
			info.LastAt, info.LastBytes, info.Encrypted = e.CreatedAt, e.Size, e.Encrypted
		}
	}
	return info
}

func readLogInfo(path string) logInfo {
	info := logInfo{Path: path, SizeBytes: -1}
	if path == "" {
		return info
	}
	if fi, err := os.Stat(path); err == nil {
		info.SizeBytes = fi.Size()
	}
	return info
}

// ── OS updates (apt) ──

var (
	aptCache    updatesInfo
	aptCacheAt  time.Time
	aptCacheMu  sync.Mutex
	aptCacheTTL = 15 * time.Minute
)

// cachedAptUpdates runs a read-only `apt-get -s upgrade` (simulation, no root
// needed) at most once per aptCacheTTL. Non-Debian hosts report "unavailable".
func cachedAptUpdates(ctx context.Context) updatesInfo {
	aptCacheMu.Lock()
	defer aptCacheMu.Unlock()
	if !aptCacheAt.IsZero() && time.Since(aptCacheAt) < aptCacheTTL {
		return aptCache
	}

	info := updatesInfo{Status: "unavailable"}
	if _, err := exec.LookPath("apt-get"); err == nil {
		cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(cctx, "apt-get", "-s", "-q", "upgrade")
		// Minimal env: don't hand bekci's own environment (BEKCI_* secrets) to a child process.
		cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "LC_ALL=C"}
		if out, err := cmd.Output(); err == nil {
			total, security := parseAptSimulation(string(out))
			info = updatesInfo{
				Status:    updatesStatus(total, security),
				Total:     total,
				Security:  security,
				CheckedAt: time.Now().UTC().Format(time.RFC3339),
			}
		} else if cctx.Err() != nil {
			return info // timed out or request cancelled: don't cache, retry next time
		}
	}
	aptCache, aptCacheAt = info, time.Now()
	return aptCache
}

// parseAptSimulation counts "Inst" lines; a package is a security update when
// any of its candidate origins is a *-security pocket.
func parseAptSimulation(out string) (total, security int) {
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "Inst ") {
			continue
		}
		total++
		if strings.Contains(line, "-security") {
			security++
		}
	}
	return total, security
}

func updatesStatus(total, security int) string {
	switch {
	case security > 0:
		return "security"
	case total > 0:
		return "updates"
	default:
		return "up_to_date"
	}
}

func round1(f float64) float64 {
	return math.Round(f*10) / 10
}
