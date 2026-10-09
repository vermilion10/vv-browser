// Package tunnel runs an OpenVPN session entirely in userspace: minivpn
// speaks the OpenVPN protocol and gVisor's netstack (via wireguard-go)
// terminates the tunnelled IP packets, so no TUN device or system VPN
// permission is needed.
package tunnel

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/apex/log"
	"github.com/ooni/minivpn/pkg/config"
	vpn "github.com/ooni/minivpn/pkg/tunnel"
	"golang.zx2c4.com/wireguard/tun"
	"golang.zx2c4.com/wireguard/tun/netstack"
)

// Tunnel is a connected OpenVPN session that can dial TCP/UDP through it.
type Tunnel struct {
	vpn  *vpn.TUN
	dev  tun.Device
	net  *netstack.Net
	ip   string
	done chan struct{}
	once sync.Once
}

// Start connects to the server described by ovpn (the contents of an
// .ovpn file) and returns once the tunnel can carry traffic. ctx bounds only
// the handshake.
func Start(ctx context.Context, ovpn []byte, mtu int) (*Tunnel, error) {
	opts, err := parseOVPN(ovpn)
	if err != nil {
		return nil, err
	}
	cfg := config.NewConfig(config.WithOpenVPNOptions(opts), config.WithLogger(log.Log))

	v, err := vpn.Start(ctx, &net.Dialer{}, cfg)
	if err != nil {
		return nil, err
	}

	local, err := netip.ParseAddr(v.LocalAddr().String())
	if err != nil {
		v.Close()
		return nil, fmt.Errorf("tunnel: bad local address %q: %w", v.LocalAddr(), err)
	}
	// minivpn does not expose pushed DNS servers; VPN Gate relays push
	// 8.8.8.8. Queries still travel through the tunnel.
	dns := []netip.Addr{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("1.1.1.1")}

	dev, tnet, err := netstack.CreateNetTUN([]netip.Addr{local}, dns, mtu)
	if err != nil {
		v.Close()
		return nil, err
	}

	t := &Tunnel{vpn: v, dev: dev, net: tnet, ip: local.String(), done: make(chan struct{})}
	go t.pumpUp()
	go t.pumpDown(mtu)
	go t.keepalive()
	return t, nil
}

// pingPayload is OpenVPN's keepalive message, sent as an ordinary data
// packet.
var pingPayload = []byte{
	0x2a, 0x18, 0x7b, 0xf3, 0x64, 0x1e, 0xb4, 0xcb,
	0x07, 0xed, 0x2d, 0x0a, 0x98, 0x1f, 0xc7, 0x48,
}

// keepalive sends OpenVPN pings, which minivpn does not do on its own.
// VPN Gate relays push "ping 3" and "ping-restart 10" and drop a client
// that stays silent past the restart window.
func (t *Tunnel) keepalive() {
	tick := time.NewTicker(3 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-t.done:
			return
		case <-tick.C:
			if _, err := t.vpn.Write(append([]byte(nil), pingPayload...)); err != nil {
				t.fail(err)
				return
			}
		}
	}
}

// pumpUp moves packets arriving from the VPN into the netstack.
// minivpn's TUN.Read returns exactly one packet per call.
func (t *Tunnel) pumpUp() {
	buf := make([]byte, 65535)
	for {
		n, err := t.vpn.Read(buf)
		if err != nil {
			t.fail(err)
			return
		}
		if _, err := t.dev.Write([][]byte{buf[:n]}, 0); err != nil {
			t.fail(err)
			return
		}
	}
}

// pumpDown moves packets produced by the netstack to the VPN. minivpn keeps
// the written slice, so each packet gets its own copy.
func (t *Tunnel) pumpDown(mtu int) {
	bufs := [][]byte{make([]byte, mtu+64)}
	sizes := []int{0}
	for {
		n, err := t.dev.Read(bufs, sizes, 0)
		if err != nil {
			t.fail(err)
			return
		}
		for i := 0; i < n; i++ {
			pkt := append([]byte(nil), bufs[i][:sizes[i]]...)
			if _, err := t.vpn.Write(pkt); err != nil {
				t.fail(err)
				return
			}
		}
	}
}

// fail reports a pump error and closes the tunnel. Errors that follow our
// own Close are expected and stay quiet.
func (t *Tunnel) fail(err error) {
	select {
	case <-t.done:
	default:
		log.WithError(err).Warn("tunnel: link down")
	}
	t.Close()
}

// DialContext dials through the tunnel. Hostnames are resolved by DNS
// queries sent through the tunnel too.
func (t *Tunnel) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return t.net.DialContext(ctx, network, address)
}

// LocalIP is the address the VPN server assigned to us.
func (t *Tunnel) LocalIP() string { return t.ip }

// Done is closed when the tunnel stops working.
func (t *Tunnel) Done() <-chan struct{} { return t.done }

// Close tears the tunnel down. It is safe to call more than once.
func (t *Tunnel) Close() error {
	t.once.Do(func() {
		close(t.done)
		t.vpn.Close()
		t.dev.Close()
	})
	return nil
}

// minivpnKeys are the directives minivpn's parser understands. Everything
// else is dropped beforehand, since the parser logs a warning for each
// unknown line and VPN Gate configs are mostly comments and client-only
// directives.
var minivpnKeys = map[string]bool{
	"proto": true, "remote": true, "cipher": true, "auth": true,
	"compress": true, "comp-lzo": true, "tls-version-max": true, "proxy-obfs4": true,
	"ca": true, "cert": true, "key": true, "auth-user-pass": true,
}

func filterOVPN(ovpn []byte) []byte {
	var out bytes.Buffer
	inline := ""
	for _, line := range strings.Split(string(ovpn), "\n") {
		l := strings.TrimSpace(line)
		switch {
		case inline != "":
			out.WriteString(l + "\n")
			if l == "</"+inline+">" {
				inline = ""
			}
		case strings.HasPrefix(l, "<") && strings.HasSuffix(l, ">") && !strings.HasPrefix(l, "</"):
			inline = strings.Trim(l, "<>")
			out.WriteString(l + "\n")
		default:
			if f := strings.Fields(l); len(f) > 0 && minivpnKeys[f[0]] {
				out.WriteString(strings.Join(f, " ") + "\n")
			}
		}
	}
	return out.Bytes()
}

// parseOVPN parses an .ovpn file's contents. Only inline certificates are
// supported, since there is no directory to resolve file paths against.
func parseOVPN(ovpn []byte) (*config.OpenVPNOptions, error) {
	opts, err := config.ReadConfig(filterOVPN(ovpn), "")
	if err != nil {
		return nil, err
	}
	if !opts.HasAuthInfo() {
		return nil, errors.New("tunnel: config has no ca/cert/key or credentials")
	}
	// VPN Gate relays accept "vpn"/"vpn" and some reject an empty login.
	if opts.Username == "" {
		opts.Username, opts.Password = "vpn", "vpn"
	}
	return opts, nil
}
