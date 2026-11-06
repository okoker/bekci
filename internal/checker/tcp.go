package checker

import (
	"fmt"
	"net"
	"strconv"
	"time"
)

// runTCP connects to host:port. Normally a connect is up and any failure is
// down. With expect_closed (a port that must stay closed) it is the other way
// round: any failure (refused, timed out, unreachable) is up, a connect is down.
func runTCP(host string, config map[string]any) *Result {
	port := configInt(config, "port", 80)
	timeoutS := configInt(config, "timeout_s", 5)
	expectClosed := configBool(config, "expect_closed", false)

	addr := net.JoinHostPort(host, strconv.Itoa(port))
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, time.Duration(timeoutS)*time.Second)
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		status, message := "down", err.Error()
		if expectClosed {
			status, message = "up", "Port closed as expected: "+err.Error()
		}
		return &Result{
			Status:     status,
			ResponseMs: elapsed,
			Message:    message,
			Metrics:    map[string]any{"addr": addr},
		}
	}
	conn.Close()

	if expectClosed {
		return &Result{
			Status:     "down",
			ResponseMs: elapsed,
			Message:    fmt.Sprintf("TCP connect to %s succeeded, but this port should be closed", addr),
			Metrics:    map[string]any{"addr": addr},
		}
	}
	return &Result{
		Status:     "up",
		ResponseMs: elapsed,
		Message:    fmt.Sprintf("TCP connect to %s OK", addr),
		Metrics:    map[string]any{"addr": addr},
	}
}
