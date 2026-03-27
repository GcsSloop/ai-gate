package netproxy

import "net/url"

func proxyURLFromParts(scheme string, host string, port string) (*url.URL, error) {
	if host == "" || port == "" {
		return nil, nil
	}
	return url.Parse(scheme + "://" + host + ":" + port)
}
