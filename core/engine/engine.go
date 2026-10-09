// Package engine wires the VPN Gate server list, the userspace tunnel and the
// routing proxy together. It is shared by the vvcore command and the mobile
// bindings.
package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"time"

	"github.com/apex/log"

	"vvbrowser/core/proxy"
	"vvbrowser/core/tunnel"
	"vvbrowser/core/vpngate"
)

// Options configure an Engine.
type Options struct {
	// Listen is the proxy address; use port 0 to pick a free port.
	Listen string
	// Country selects VPN Gate relays (ISO 3166 alpha-2).
	Country string
	// OVPN, if set, is used instead of the VPN Gate server list.
	OVPN []byte
	// OVPNName labels the OVPN config in logs.
	OVPNName string
	// MTU of the userspace tunnel interface.
	MTU int
	// StateDir, if set, keeps state across runs (the preferred relay).
	StateDir string
}

func (o *Options) setDefaults() {
	if o.Listen == "" {
		o.Listen = "127.0.0.1:0"
	}
	if o.Country == "" {
		o.Country = "JP"
	}
	if o.MTU == 0 {
		o.MTU = 1400
	}
	if o.OVPNName == "" {
		o.OVPNName = "custom.ovpn"
	}
}

// Engine is a running local proxy backed by a tunnel manager.
type Engine struct {
	Tunnel *tunnel.Manager

	srv *http.Server
	ln  net.Listener
}

// New prepares an engine without connecting or listening yet.
func New(opts Options) *Engine {
	opts.setDefaults()
	mgr := &tunnel.Manager{
		Candidates:     candidates(opts),
		MTU:            opts.MTU,
		AttemptTimeout: 20 * time.Second,
		MaxAttempts:    8,
	}
	// A custom config always wins, so only the VPN Gate mode remembers relays.
	if opts.StateDir != "" && len(opts.OVPN) == 0 {
		mgr.StatePath = filepath.Join(opts.StateDir, "relay.json")
	}
	return &Engine{
		Tunnel: mgr,
		srv: &http.Server{
			Addr: opts.Listen,
			Handler: &proxy.Server{
				Rules:  proxy.DefaultRules(),
				Direct: &net.Dialer{Timeout: 15 * time.Second},
				Tunnel: mgr,
			},
		},
	}
}

// Connect brings the tunnel up so the first proxied request does not wait
// on the handshake.
func (e *Engine) Connect(ctx context.Context) error {
	_, err := e.Tunnel.Get(ctx)
	return err
}

// Listen binds the proxy and starts serving in the background. It returns
// the bound address.
func (e *Engine) Listen() (string, error) {
	ln, err := net.Listen("tcp", e.srv.Addr)
	if err != nil {
		return "", err
	}
	e.ln = ln
	go func() {
		if err := e.srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.WithError(err).Error("proxy stopped")
		}
	}()
	log.Infof("proxy listening on http://%s", ln.Addr())
	return ln.Addr().String(), nil
}

// Close stops the proxy and the tunnel.
func (e *Engine) Close() error {
	err := e.srv.Close()
	e.Tunnel.Close()
	return err
}

func candidates(opts Options) func(context.Context) ([]tunnel.Candidate, error) {
	if len(opts.OVPN) > 0 {
		c := tunnel.Candidate{ID: "ovpn:" + opts.OVPNName, Name: opts.OVPNName, Config: opts.OVPN}
		return func(context.Context) ([]tunnel.Candidate, error) {
			return []tunnel.Candidate{c}, nil
		}
	}
	return func(ctx context.Context) ([]tunnel.Candidate, error) {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		all, err := vpngate.Fetch(ctx, http.DefaultClient)
		if err != nil {
			return nil, err
		}
		servers := vpngate.InCountry(all, opts.Country)
		log.Infof("vpngate: %d servers in %s (of %d)", len(servers), opts.Country, len(all))
		out := make([]tunnel.Candidate, 0, len(servers))
		for _, s := range servers {
			out = append(out, tunnel.Candidate{
				ID:   "vpngate:" + s.HostName,
				Name:   fmt.Sprintf("%s (%s, %d ms, %.0f Mbps)", s.HostName, s.IP, s.PingMS, float64(s.SpeedBps)/1e6),
				Config: s.Config,
			})
		}
		return out, nil
	}
}

// ExitInfo fetches ipinfo.io through the tunnel and returns the JSON body.
func (e *Engine) ExitInfo(ctx context.Context) (string, error) {
	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: &http.Transport{DialContext: e.Tunnel.DialContext},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://ipinfo.io/json", nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	return string(b), err
}
