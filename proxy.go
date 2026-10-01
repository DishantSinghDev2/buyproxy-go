package buyproxy

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Protocol is the proxy protocol: "http" or "socks5".
type Protocol string

const (
	HTTP   Protocol = "http"
	SOCKS5 Protocol = "socks5"
)

// Proxy is one generated proxy. The API returns username:password@host:port lines.
type Proxy struct {
	Host     string
	Port     int
	Username string
	Password string
	Protocol Protocol
}

// ParseProxy parses a username:password@host:port line. '@' and ':' are allowed inside the password.
func ParseProxy(line string, protocol Protocol) (Proxy, error) {
	bad := fmt.Errorf("buyproxy: not a proxy line (expected username:password@host:port): %q", line)
	at := strings.LastIndex(line, "@")
	if at < 0 {
		return Proxy{}, bad
	}
	credentials, address := line[:at], line[at+1:]
	colon := strings.Index(credentials, ":")
	lastColon := strings.LastIndex(address, ":")
	if colon < 1 || lastColon < 1 {
		return Proxy{}, bad
	}
	port, err := strconv.Atoi(address[lastColon+1:])
	if err != nil || port < 1 || port > 65535 {
		return Proxy{}, bad
	}
	if protocol == "" {
		protocol = HTTP
	}
	return Proxy{Host: address[:lastColon], Port: port, Username: credentials[:colon], Password: credentials[colon+1:], Protocol: protocol}, nil
}

// String returns the username:password@host:port form the API uses.
func (p Proxy) String() string {
	return fmt.Sprintf("%s:%s@%s:%d", p.Username, p.Password, p.Host, p.Port)
}

// URL returns the proxy as a *url.URL with credentials, ready for http.ProxyURL.
func (p Proxy) URL() *url.URL {
	return &url.URL{Scheme: string(p.Protocol), User: url.UserPassword(p.Username, p.Password), Host: fmt.Sprintf("%s:%d", p.Host, p.Port)}
}

// Transport returns an *http.Transport that sends every request through this proxy (HTTP or SOCKS5).
func (p Proxy) Transport() *http.Transport {
	return &http.Transport{Proxy: http.ProxyURL(p.URL())}
}

// HTTPClient returns an *http.Client that sends every request through this proxy.
func (p Proxy) HTTPClient() *http.Client {
	return &http.Client{Transport: p.Transport()}
}

// Env returns the environment variables understood by curl, wget, git and most CLIs.
func (p Proxy) Env() map[string]string {
	u := p.URL().String()
	return map[string]string{"HTTP_PROXY": u, "HTTPS_PROXY": u, "ALL_PROXY": u, "http_proxy": u, "https_proxy": u, "all_proxy": u}
}

// Curl returns a ready-to-run curl command that prints your exit IP through this proxy.
func (p Proxy) Curl() string {
	scheme := "http"
	if p.Protocol == SOCKS5 {
		scheme = "socks5h"
	}
	return fmt.Sprintf("curl -x '%s://%s:%s@%s:%d' https://api.ipify.org", scheme, p.Username, p.Password, p.Host, p.Port)
}

// GeneratedProxies is the result of a Generate call: the raw lines plus parsed proxies.
type GeneratedProxies struct {
	Lines   []string
	Proxies []Proxy
}

func newGenerated(lines []string, protocol Protocol) (*GeneratedProxies, error) {
	out := &GeneratedProxies{Lines: lines, Proxies: make([]Proxy, 0, len(lines))}
	for _, line := range lines {
		p, err := ParseProxy(line, protocol)
		if err != nil {
			return nil, err
		}
		out.Proxies = append(out.Proxies, p)
	}
	return out, nil
}
