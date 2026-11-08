package checker

import (
	"net"
	"strconv"
	"strings"
)

// buildHTTPURL builds the URL for the HTTP and Page Hash checks from the
// target Host (a name or IP, optionally followed by a path; SplitHost), the
// check's port and its Endpoint. IPv6 names are bracketed. An Endpoint of ""
// or "/" adds nothing to the Host's path (with no path, "" leaves the URL at
// the bare host and "/" asks for the root). Any other Endpoint goes after the
// Host's path with exactly one "/" between them, so "health", "/health" and
// "//health" all mean /health. Mirrored by checkURL in
// frontend/src/conditionRules.js; both are pinned by testdata/http_urls.json.
func buildHTTPURL(scheme, host string, port int, endpoint string) string {
	name, path := SplitHost(host)
	hostPart := name
	if port > 0 {
		hostPart = net.JoinHostPort(name, strconv.Itoa(port))
	} else if strings.Contains(name, ":") {
		hostPart = "[" + name + "]"
	}
	base := scheme + "://" + hostPart
	if endpoint == "" || endpoint == "/" {
		if path != "" {
			return base + path
		}
		if endpoint == "" {
			return base
		}
		return base + "/"
	}
	return base + strings.TrimRight(path, "/") + "/" + strings.TrimLeft(endpoint, "/")
}
