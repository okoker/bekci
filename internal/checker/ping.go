package checker

import (
	"fmt"
	"time"

	probing "github.com/prometheus-community/pro-bing"
)

func runPing(host string, config map[string]any) *Result {
	count := configInt(config, "count", 3)
	timeoutS := configInt(config, "timeout_s", 5)

	pinger, err := probing.NewPinger(host)
	if err != nil {
		return &Result{
			Status:  "down",
			Message: fmt.Sprintf("pinger init: %v", err),
			Metrics: map[string]any{},
		}
	}
	pinger.Count = count
	pinger.Timeout = time.Duration(timeoutS) * time.Second
	pinger.SetPrivileged(true) // requires NET_RAW capability

	start := time.Now()
	if err := pinger.Run(); err != nil {
		return &Result{
			Status:     "down",
			ResponseMs: time.Since(start).Milliseconds(),
			Message:    fmt.Sprintf("ping failed: %v", err),
			Metrics:    map[string]any{},
		}
	}

	stats := pinger.Statistics()
	elapsed := time.Since(start).Milliseconds()
	packetLoss := stats.PacketLoss
	avgRtt := float64(stats.AvgRtt.Milliseconds())

	status := "up"
	msg := fmt.Sprintf("%d/%d packets received, avg %.1fms", stats.PacketsRecv, stats.PacketsSent, avgRtt)
	if stats.PacketsRecv == 0 {
		status = "down"
		msg = "100% packet loss"
	}

	return &Result{
		Status:     status,
		ResponseMs: elapsed,
		Message:    msg,
		Metrics: map[string]any{
			"packet_loss": packetLoss,
			"avg_rtt_ms":  avgRtt,
			"packets_sent": stats.PacketsSent,
			"packets_recv": stats.PacketsRecv,
		},
	}
}
