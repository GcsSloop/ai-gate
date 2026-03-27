//go:build !darwin && !windows

package netproxy

import (
	"net/http"
	"net/url"
)

func resolveSystemProxy(req *http.Request) (*url.URL, error) {
	return nil, nil
}
