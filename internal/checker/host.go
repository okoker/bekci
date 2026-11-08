package checker

import (
	"net"
	"strings"
	"unicode"
)

// A target Host is a name or IP address, optionally followed by a path
// (www.example.com/health). HTTP and Page Hash checks request that path
// (buildHTTPURL); every other check uses the name only (Run).

// SplitHost splits a target Host at its first "/" into the name (or IP) and
// the path, which keeps its leading "/" and is returned as typed.
func SplitHost(host string) (name, path string) {
	if i := strings.Index(host, "/"); i >= 0 {
		return host[:i], host[i:]
	}
	return host, ""
}

// HostError returns why Save refuses a target Host, or "" when it is fine.
// The host is expected already trimmed; an empty host is left to the
// "required" check. Mirrored by hostError in frontend/src/conditionRules.js;
// both are pinned by testdata/host_rules.json.
func HostError(host string) string {
	if host == "" {
		return ""
	}
	// Spaces, control characters and invisible format characters (a pasted
	// byte-order mark or zero-width space) would all break the check's address.
	if strings.IndexFunc(host, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r) || unicode.Is(unicode.Cf, r)
	}) >= 0 {
		return "Host can't contain spaces or invisible characters."
	}
	if strings.Contains(host, "://") {
		return "Leave out http:// or https://: each HTTP or Page Hash check has its own Scheme."
	}
	if strings.ContainsAny(host, "?#") {
		return "Put a query (? or #) in the check's Endpoint, not in Host."
	}
	name, _ := SplitHost(host)
	if name == "" {
		return "Host needs a name or IP address before the path."
	}
	if strings.Contains(name, "@") {
		return "Host can't contain @ before the path."
	}
	if strings.Contains(name, ":") && net.ParseIP(name) == nil {
		return "Leave out the port: each check has its own Port field. Write an IPv6 address without brackets."
	}
	return ""
}
