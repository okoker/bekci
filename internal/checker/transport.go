package checker

import (
	"crypto/tls"
	"net/http"
	"time"
)

var (
	sharedTransport = &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 5,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		TLSClientConfig: &tls.Config{
			ClientSessionCache: tls.NewLRUClientSessionCache(128),
		},
	}
	skipTLSTransport = &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 5,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify:  true,
			ClientSessionCache: tls.NewLRUClientSessionCache(128),
		},
	}
)
