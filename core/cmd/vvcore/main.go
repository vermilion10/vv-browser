// Command vvcore runs the VV Browser network core as a standalone local
// proxy: an in-process OpenVPN tunnel to a Japanese VPN Gate relay, plus an
// HTTP proxy that routes only region-checked hosts through it.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/apex/log"
	"github.com/apex/log/handlers/text"

	"vvbrowser/core/engine"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:8899", "address for the local HTTP proxy")
	ovpn := flag.String("ovpn", "", "use this .ovpn file instead of the VPN Gate server list")
	country := flag.String("country", "JP", "VPN Gate country code to pick servers from")
	mtu := flag.Int("mtu", 1400, "MTU of the userspace tunnel interface")
	check := flag.Bool("check", false, "connect, print the tunnel's exit IP info, and exit")
	verbose := flag.Bool("v", false, "verbose OpenVPN logging")
	stateDir := flag.String("state-dir", "", "directory for state kept across runs (the preferred relay)")
	flag.Parse()

	log.SetHandler(text.New(os.Stderr))
	log.SetLevel(log.InfoLevel)
	if *verbose {
		log.SetLevel(log.DebugLevel)
	}

	opts := engine.Options{Listen: *listen, Country: *country, MTU: *mtu, StateDir: *stateDir}
	if *ovpn != "" {
		b, err := os.ReadFile(*ovpn)
		if err != nil {
			log.WithError(err).Fatal("cannot read config")
		}
		opts.OVPN, opts.OVPNName = b, filepath.Base(*ovpn)
	}
	eng := engine.New(opts)
	defer eng.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := eng.Connect(ctx); err != nil {
		log.WithError(err).Fatal("could not bring the tunnel up")
	}

	if *check {
		start := time.Now()
		info, err := eng.ExitInfo(ctx)
		if err != nil {
			log.WithError(err).Fatal("check failed")
		}
		fmt.Printf("%s\n(request took %v)\n", info, time.Since(start).Round(time.Millisecond))
		return
	}

	if _, err := eng.Listen(); err != nil {
		log.WithError(err).Fatal("cannot listen")
	}
	<-ctx.Done()
}
