package checker

import (
	"fmt"
	"net"
	"time"
)

func runTCP(host string, config map[string]any) *Result {
	port := configInt(config, "port", 80)
	timeoutS := configInt(config, "timeout_s", 5)

	addr := fmt.Sprintf("%s:%d", host, port)
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, time.Duration(timeoutS)*time.Second)
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		return &Result{
			Status:     "down",
			ResponseMs: elapsed,
			Message:    err.Error(),
			Metrics:    map[string]any{"addr": addr},
		}
	}
	conn.Close()

	return &Result{
		Status:     "up",
		ResponseMs: elapsed,
		Message:    fmt.Sprintf("TCP connect to %s OK", addr),
		Metrics:    map[string]any{"addr": addr},
	}
}
