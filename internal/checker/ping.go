package checker

import (
	"fmt"
	"math"
	"os"
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

	// Use privileged (raw ICMP) if running as root or in Docker with NET_RAW,
	// otherwise use unprivileged (UDP) mode which works without special caps.
	pinger.SetPrivileged(os.Getuid() == 0)

	start := time.Now()
	if err := pinger.Run(); err != nil {
		return &Result{
			Status:     "down",
			ResponseMs: time.Since(start).Milliseconds(),
			Message:    fmt.Sprintf("ping failed: %v", err),
			Metrics:    map[string]any{},
		}
	}

	return pingResult(pinger.Statistics(), time.Since(start))
}

// pingResult turns pro-bing's statistics into a Result. The response time is
// the average round trip: pro-bing times each packet from its own send to its
// own reply and averages only the replies that came back. The run itself is
// not used while a reply came back, because it is far longer (one packet a
// second, and on to the timeout when a packet is lost). With no reply there is
// no round trip, so it is the time until the check gave up, as for the other
// check types. avg_rtt_ms keeps tenths of a millisecond.
func pingResult(stats *probing.Statistics, runTime time.Duration) *Result {
	packetLoss := stats.PacketLoss
	avgRtt := math.Round(float64(stats.AvgRtt)/float64(time.Millisecond)*10) / 10

	status := "up"
	// At least 1 ms: Run replaces a ResponseMs of 0 with the whole run time.
	responseMs := max(1, int64(math.Round(avgRtt)))
	msg := fmt.Sprintf("%d/%d packets received, avg %.1fms", stats.PacketsRecv, stats.PacketsSent, avgRtt)
	if stats.PacketsRecv == 0 {
		status = "down"
		responseMs = runTime.Milliseconds()
		msg = "100% packet loss"
	}

	return &Result{
		Status:     status,
		ResponseMs: responseMs,
		Message:    msg,
		Metrics: map[string]any{
			"packet_loss":  packetLoss,
			"avg_rtt_ms":   avgRtt,
			"packets_sent": stats.PacketsSent,
			"packets_recv": stats.PacketsRecv,
		},
	}
}
