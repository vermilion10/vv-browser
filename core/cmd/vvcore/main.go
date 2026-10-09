// Command vvcore runs the VV Browser network core as a standalone local
// proxy: an in-process OpenVPN tunnel to a Japanese VPN Gate relay, plus an
// HTTP proxy that routes only region-checked hosts through it.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/apex/log"
	"github.com/apex/log/handlers/text"

	"vvbrowser/core/proxy"
	"vvbrowser/core/tunnel"
	"vvbrowser/core/vpngate"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:8899", "address for the local HTTP proxy")
	ovpn := flag.String("ovpn", "", "use this .ovpn file instead of the VPN Gate server list")
	country := flag.String("country", "JP", "VPN Gate country code to pick servers from")
	mtu := flag.Int("mtu", 1400, "MTU of the userspace tunnel interface")
	check := flag.Bool("check", false, "connect, print the tunnel's exit IP info, and exit")
	verbose := flag.Bool("v", false, "verbose OpenVPN logging")
	flag.Parse()

	log.SetHandler(text.New(os.Stderr))
	log.SetLevel(log.InfoLevel)
	if *verbose {
		log.SetLevel(log.DebugLevel)
	}

	mgr := &tunnel.Manager{
		Candidates:     candidates(*ovpn, *country),
		MTU:            *mtu,
		AttemptTimeout: 20 * time.Second,
		MaxAttempts:    8,
	}
	defer mgr.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if *check {
		if err := runCheck(ctx, mgr); err != nil {
			log.WithError(err).Fatal("check failed")
		}
		return
	}

	// Connect up front so the first page load does not wait on the handshake.
	if _, err := mgr.Get(ctx); err != nil {
		log.WithError(err).Fatal("could not bring the tunnel up")
	}

	srv := &http.Server{
		Addr: *listen,
		Handler: &proxy.Server{
			Rules:  proxy.DefaultRules(),
			Direct: &net.Dialer{Timeout: 15 * time.Second},
			Tunnel: mgr,
		},
	}
	go func() {
		<-ctx.Done()
		srv.Close()
	}()
	log.Infof("proxy listening on http://%s", *listen)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.WithError(err).Fatal("proxy stopped")
	}
}

func candidates(ovpnPath, country string) func(context.Context) ([]tunnel.Candidate, error) {
	if ovpnPath != "" {
		return func(context.Context) ([]tunnel.Candidate, error) {
			b, err := os.ReadFile(ovpnPath)
			if err != nil {
				return nil, err
			}
			return []tunnel.Candidate{{Name: filepath.Base(ovpnPath), Config: b}}, nil
		}
	}
	return func(ctx context.Context) ([]tunnel.Candidate, error) {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		all, err := vpngate.Fetch(ctx, http.DefaultClient)
		if err != nil {
			return nil, err
		}
		servers := vpngate.InCountry(all, country)
		log.Infof("vpngate: %d servers in %s (of %d)", len(servers), country, len(all))
		out := make([]tunnel.Candidate, 0, len(servers))
		for _, s := range servers {
			out = append(out, tunnel.Candidate{
				Name:   fmt.Sprintf("%s (%s, %d ms, %.0f Mbps)", s.HostName, s.IP, s.PingMS, float64(s.SpeedBps)/1e6),
				Config: s.Config,
			})
		}
		return out, nil
	}
}

func runCheck(ctx context.Context, mgr *tunnel.Manager) error {
	if _, err := mgr.Get(ctx); err != nil {
		return err
	}
	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: &http.Transport{DialContext: mgr.DialContext},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://ipinfo.io/json", nil)
	if err != nil {
		return err
	}
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	fmt.Printf("%s\n(request took %v)\n", body, time.Since(start).Round(time.Millisecond))
	return nil
}
