package checker

import (
	"fmt"
	"time"

	"github.com/gosnmp/gosnmp"
)

// System OIDs — always queried
var snmpSystemOIDs = []string{
	"1.3.6.1.2.1.1.1.0", // sysDescr
	"1.3.6.1.2.1.1.3.0", // sysUpTime (hundredths of seconds)
	"1.3.6.1.2.1.1.4.0", // sysContact
	"1.3.6.1.2.1.1.5.0", // sysName
}

// Best-effort OIDs
const (
	oidHrProcessorLoad = "1.3.6.1.2.1.25.3.3.1.2" // walk for CPU % per core
	oidHrMemorySize    = "1.3.6.1.2.1.25.2.2.0"    // total RAM in KB
)

func runSNMPv2c(host string, config map[string]any) *Result {
	port := uint16(configInt(config, "port", 161))
	timeoutS := configInt(config, "timeout_s", 5)
	community := configStr(config, "community", "public")

	g := &gosnmp.GoSNMP{
		Target:    host,
		Port:      port,
		Version:   gosnmp.Version2c,
		Community: community,
		Timeout:   time.Duration(timeoutS) * time.Second,
		Retries:   1,
	}

	return doSNMPCheck(g, host, port)
}

func runSNMPv3(host string, config map[string]any) *Result {
	port := uint16(configInt(config, "port", 161))
	timeoutS := configInt(config, "timeout_s", 5)
	username := configStr(config, "username", "")
	secLevel := configStr(config, "security_level", "authPriv")
	authProto := configStr(config, "auth_protocol", "SHA")
	authPass := configStr(config, "auth_passphrase", "")
	privProto := configStr(config, "privacy_protocol", "AES")
	privPass := configStr(config, "privacy_passphrase", "")

	if username == "" {
		return &Result{
			Status:  "down",
			Message: "SNMP v3 username not configured (check Settings)",
			Metrics: map[string]any{},
		}
	}

	msgFlags := gosnmp.AuthPriv
	switch secLevel {
	case "noAuthNoPriv":
		msgFlags = gosnmp.NoAuthNoPriv
	case "authNoPriv":
		msgFlags = gosnmp.AuthNoPriv
	default:
		msgFlags = gosnmp.AuthPriv
	}

	sp := &gosnmp.UsmSecurityParameters{
		UserName: username,
	}

	if msgFlags >= gosnmp.AuthNoPriv {
		switch authProto {
		case "MD5":
			sp.AuthenticationProtocol = gosnmp.MD5
		default:
			sp.AuthenticationProtocol = gosnmp.SHA
		}
		sp.AuthenticationPassphrase = authPass
	}

	if msgFlags >= gosnmp.AuthPriv {
		switch privProto {
		case "DES":
			sp.PrivacyProtocol = gosnmp.DES
		default:
			sp.PrivacyProtocol = gosnmp.AES
		}
		sp.PrivacyPassphrase = privPass
	}

	g := &gosnmp.GoSNMP{
		Target:             host,
		Port:               port,
		Version:            gosnmp.Version3,
		SecurityModel:      gosnmp.UserSecurityModel,
		MsgFlags:           msgFlags,
		SecurityParameters: sp,
		Timeout:            time.Duration(timeoutS) * time.Second,
		Retries:            1,
	}

	return doSNMPCheck(g, host, port)
}

func doSNMPCheck(g *gosnmp.GoSNMP, host string, port uint16) *Result {
	start := time.Now()

	if err := g.Connect(); err != nil {
		return &Result{
			Status:     "down",
			ResponseMs: time.Since(start).Milliseconds(),
			Message:    fmt.Sprintf("SNMP connect failed: %v", err),
			Metrics:    map[string]any{},
		}
	}
	defer g.Conn.Close()

	// Query system OIDs
	result, err := g.Get(snmpSystemOIDs)
	if err != nil {
		return &Result{
			Status:     "down",
			ResponseMs: time.Since(start).Milliseconds(),
			Message:    fmt.Sprintf("SNMP GET failed: %v", err),
			Metrics:    map[string]any{},
		}
	}

	elapsed := time.Since(start).Milliseconds()
	metrics := map[string]any{}

	for _, pdu := range result.Variables {
		switch pdu.Name {
		case ".1.3.6.1.2.1.1.1.0":
			metrics["sys_descr"] = pduToString(pdu)
		case ".1.3.6.1.2.1.1.3.0":
			if v, ok := pdu.Value.(uint32); ok {
				metrics["sys_uptime_s"] = int64(v) / 100
			}
		case ".1.3.6.1.2.1.1.4.0":
			metrics["sys_contact"] = pduToString(pdu)
		case ".1.3.6.1.2.1.1.5.0":
			metrics["sys_name"] = pduToString(pdu)
		}
	}

	// Best-effort: CPU (walk hrProcessorLoad)
	cpuAvg := snmpWalkCPU(g)
	if cpuAvg >= 0 {
		metrics["cpu_avg_pct"] = cpuAvg
	}

	// Best-effort: memory
	memKB := snmpGetMemory(g)
	if memKB > 0 {
		metrics["memory_total_kb"] = memKB
	}

	sysName, _ := metrics["sys_name"].(string)
	msg := fmt.Sprintf("SNMP OK — %s:%d", host, port)
	if sysName != "" {
		msg = fmt.Sprintf("SNMP OK — %s (%s:%d)", sysName, host, port)
	}

	return &Result{
		Status:     "up",
		ResponseMs: elapsed,
		Message:    msg,
		Metrics:    metrics,
	}
}

func snmpWalkCPU(g *gosnmp.GoSNMP) int {
	results, err := g.BulkWalkAll(oidHrProcessorLoad)
	if err != nil || len(results) == 0 {
		return -1
	}
	var total, count int
	for _, pdu := range results {
		if v, ok := pdu.Value.(int); ok {
			total += v
			count++
		}
	}
	if count == 0 {
		return -1
	}
	return total / count
}

func snmpGetMemory(g *gosnmp.GoSNMP) int64 {
	result, err := g.Get([]string{oidHrMemorySize})
	if err != nil || len(result.Variables) == 0 {
		return 0
	}
	pdu := result.Variables[0]
	switch v := pdu.Value.(type) {
	case int:
		return int64(v)
	case uint32:
		return int64(v)
	case int64:
		return v
	}
	return 0
}

func pduToString(pdu gosnmp.SnmpPDU) string {
	switch v := pdu.Value.(type) {
	case []byte:
		return string(v)
	case string:
		return v
	default:
		return fmt.Sprintf("%v", v)
	}
}
