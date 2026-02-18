package checker

import (
	"encoding/json"
	"time"
)

// Result returned by every check type.
type Result struct {
	Status     string         `json:"status"` // "up" or "down"
	ResponseMs int64          `json:"response_ms"`
	Message    string         `json:"message"`
	Metrics    map[string]any `json:"metrics"`
}

// Run dispatches to the correct check type and returns the result.
func Run(checkType, host string, configJSON string) *Result {
	config := make(map[string]any)
	if configJSON != "" && configJSON != "{}" {
		_ = json.Unmarshal([]byte(configJSON), &config)
	}

	start := time.Now()
	var r *Result

	switch checkType {
	case "http":
		r = runHTTP(host, config)
	case "tcp":
		r = runTCP(host, config)
	case "ping":
		r = runPing(host, config)
	case "dns":
		r = runDNS(host, config)
	case "page_hash":
		r = runPageHash(host, config)
	case "tls_cert":
		r = runTLSCert(host, config)
	default:
		r = &Result{
			Status:  "down",
			Message: "unknown check type: " + checkType,
			Metrics: map[string]any{},
		}
	}

	if r.ResponseMs == 0 {
		r.ResponseMs = time.Since(start).Milliseconds()
	}
	if r.Metrics == nil {
		r.Metrics = map[string]any{}
	}
	return r
}

// helper to get a string config value with a default.
func configStr(config map[string]any, key, defaultVal string) string {
	if v, ok := config[key]; ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return defaultVal
}

// helper to get an int config value with a default.
func configInt(config map[string]any, key string, defaultVal int) int {
	if v, ok := config[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		}
	}
	return defaultVal
}

// helper to get a bool config value with a default.
func configBool(config map[string]any, key string, defaultVal bool) bool {
	if v, ok := config[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return defaultVal
}
