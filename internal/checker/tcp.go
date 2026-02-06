package checker

import (
	"fmt"
	"net"
	"time"

	"github.com/bekci/internal/config"
)

func (c *Checker) checkTCP(svc *config.Service) *Result {
	start := time.Now()

	addr := svc.URL
	if addr == "" {
		return resultDown("url is required for tcp check", 0)
	}

	conn, err := net.DialTimeout("tcp", addr, svc.Check.Timeout)
	responseMs := measureTime(start)

	if err != nil {
		return resultDown(fmt.Sprintf("tcp connect failed: %v", err), responseMs)
	}
	conn.Close()

	return resultUp(0, responseMs)
}
