//go:build darwin

package netproxy

import (
	"bufio"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
)

func resolveSystemProxy(req *http.Request) (*url.URL, error) {
	cmd := exec.Command("scutil", "--proxy")
	output, err := cmd.Output()
	if err != nil {
		return nil, nil
	}
	values := parseScutilProxyOutput(string(output))
	if strings.EqualFold(req.URL.Scheme, "https") {
		if proxyURL, err := proxyFromScutil(values, "HTTPS"); proxyURL != nil || err != nil {
			return proxyURL, err
		}
	}
	return proxyFromScutil(values, "HTTP")
}

func parseScutilProxyOutput(raw string) map[string]string {
	values := map[string]string{}
	scanner := bufio.NewScanner(strings.NewReader(raw))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.Contains(line, " : ") {
			continue
		}
		parts := strings.SplitN(line, " : ", 2)
		if len(parts) != 2 {
			continue
		}
		values[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	return values
}

func proxyFromScutil(values map[string]string, prefix string) (*url.URL, error) {
	if values[prefix+"Enable"] != "1" {
		return nil, nil
	}
	return proxyURLFromParts("http", values[prefix+"Proxy"], values[prefix+"Port"])
}
