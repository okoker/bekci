package checker

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"
)

func runDNS(host string, config map[string]any) *Result {
	query := configStr(config, "query", host)
	recordType := configStr(config, "record_type", "A")
	expectValue := configStr(config, "expect_value", "")
	nameserver := configStr(config, "nameserver", "")
	timeoutS := configInt(config, "timeout_s", 5)

	var resolver *net.Resolver
	if nameserver != "" {
		if !strings.Contains(nameserver, ":") {
			nameserver += ":53"
		}
		resolver = &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{Timeout: time.Duration(timeoutS) * time.Second}
				return d.DialContext(ctx, "udp", nameserver)
			},
		}
	} else {
		resolver = net.DefaultResolver
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutS)*time.Second)
	defer cancel()

	start := time.Now()
	var resolved []string
	var err error

	switch strings.ToUpper(recordType) {
	case "A":
		var ips []net.IP
		ips, err = resolver.LookupIP(ctx, "ip4", query)
		if err == nil {
			for _, ip := range ips {
				resolved = append(resolved, ip.String())
			}
		}
	case "AAAA":
		var ips []net.IP
		ips, err = resolver.LookupIP(ctx, "ip6", query)
		if err == nil {
			for _, ip := range ips {
				resolved = append(resolved, ip.String())
			}
		}
	case "MX":
		var mxs []*net.MX
		mxs, err = resolver.LookupMX(ctx, query)
		if err == nil {
			for _, mx := range mxs {
				resolved = append(resolved, mx.Host)
			}
		}
	case "CNAME":
		var cname string
		cname, err = resolver.LookupCNAME(ctx, query)
		if err == nil {
			resolved = append(resolved, cname)
		}
	default:
		return &Result{
			Status:  "down",
			Message: "unsupported record type: " + recordType,
			Metrics: map[string]any{},
		}
	}

	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		return &Result{
			Status:     "down",
			ResponseMs: elapsed,
			Message:    fmt.Sprintf("DNS lookup failed: %v", err),
			Metrics:    map[string]any{"query": query, "record_type": recordType},
		}
	}

	status := "up"
	msg := fmt.Sprintf("resolved: %s", strings.Join(resolved, ", "))

	if expectValue != "" {
		found := false
		for _, v := range resolved {
			if strings.TrimSuffix(v, ".") == strings.TrimSuffix(expectValue, ".") {
				found = true
				break
			}
		}
		if !found {
			status = "down"
			msg = fmt.Sprintf("expected %s, got %s", expectValue, strings.Join(resolved, ", "))
		}
	}

	return &Result{
		Status:     status,
		ResponseMs: elapsed,
		Message:    msg,
		Metrics: map[string]any{
			"query":       query,
			"record_type": recordType,
			"resolved":    resolved,
		},
	}
}
