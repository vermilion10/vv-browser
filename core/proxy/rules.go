package proxy

import (
	"net"
	"path"
	"strings"
)

// Route says how a host is reached.
type Route int

const (
	Direct Route = iota
	Tunnel
)

func (r Route) String() string {
	if r == Tunnel {
		return "tunnel"
	}
	return "direct"
}

// Rule maps a host pattern to a route. Patterns use path.Match globs, and
// "*.example.com" also matches "example.com" itself.
type Rule struct {
	Pattern string
	Route   Route
}

// Rules are checked in order; the first match wins and unmatched hosts go
// direct.
type Rules []Rule

// DefaultRules send only DMM and the Ubitus login/session host through
// Japan. The Ubitus stream servers (gc-*.ugamenow.com) do not check the
// client's region, so the video stays on the shorter direct path.
func DefaultRules() Rules {
	return Rules{
		{"gc-*.ugamenow.com", Direct},
		{"*.dmm.com", Tunnel},
		{"*.dmm.co.jp", Tunnel},
		{"*.dmmapis.com", Tunnel},
		{"dcgp-game.ugamenow.com", Tunnel},
		{"ipinfo.io", Tunnel}, // lets users confirm the tunnel's exit country
	}
}

// Match returns the route for host, which may include a port.
func (rs Rules) Match(host string) Route {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, r := range rs {
		p := strings.ToLower(r.Pattern)
		if ok, _ := path.Match(p, host); ok {
			return r.Route
		}
		if strings.HasPrefix(p, "*.") && host == p[2:] {
			return r.Route
		}
	}
	return Direct
}
