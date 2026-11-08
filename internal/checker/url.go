package checker

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// buildHTTPURL builds the URL for the HTTP and Page Hash checks. IPv6 hosts are
// bracketed. A non-empty endpoint gets exactly one leading "/" (any extra
// leading slashes are dropped, so "health", "/health" and "//health" all mean
// /health); an empty endpoint leaves the URL at the root. A trailing "/" on the
// host is dropped so it cannot double up with the endpoint's slash.
func buildHTTPURL(scheme, host string, port int, endpoint string) string {
	hostPart := host
	if strings.Contains(host, ":") {
		hostPart = "[" + host + "]"
	}
	base := fmt.Sprintf("%s://%s", scheme, hostPart)
	if port > 0 {
		base = fmt.Sprintf("%s://%s", scheme, net.JoinHostPort(host, strconv.Itoa(port)))
	}
	base = strings.TrimRight(base, "/")
	if endpoint == "" {
		return base
	}
	return base + "/" + strings.TrimLeft(endpoint, "/")
}
