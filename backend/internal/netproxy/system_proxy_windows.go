//go:build windows

package netproxy

import (
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/sys/windows/registry"
)

func resolveSystemProxy(req *http.Request) (*url.URL, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Internet Settings`, registry.QUERY_VALUE)
	if err != nil {
		return nil, nil
	}
	defer key.Close()

	enabled, _, err := key.GetIntegerValue("ProxyEnable")
	if err != nil || enabled == 0 {
		return nil, nil
	}
	server, _, err := key.GetStringValue("ProxyServer")
	if err != nil {
		return nil, nil
	}
	return parseWindowsProxyServer(req, server)
}

func parseWindowsProxyServer(req *http.Request, raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	entries := strings.Split(raw, ";")
	perScheme := map[string]string{}
	defaultEntry := ""
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if strings.Contains(entry, "=") {
			parts := strings.SplitN(entry, "=", 2)
			perScheme[strings.ToLower(strings.TrimSpace(parts[0]))] = strings.TrimSpace(parts[1])
			continue
		}
		defaultEntry = entry
	}
	selected := defaultEntry
	if value, ok := perScheme[strings.ToLower(req.URL.Scheme)]; ok {
		selected = value
	}
	if selected == "" {
		return nil, nil
	}
	if strings.Contains(selected, "://") {
		return url.Parse(selected)
	}
	return url.Parse("http://" + selected)
}
