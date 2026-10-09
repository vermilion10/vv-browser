// Package proxy is a local HTTP proxy that sends each host either direct or
// through the VPN tunnel according to Rules.
package proxy

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"

	"github.com/apex/log"
)

// Dialer opens outbound connections.
type Dialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

// Server handles CONNECT (HTTPS and WebSocket traffic) and plain
// absolute-URI HTTP requests.
type Server struct {
	Rules  Rules
	Direct Dialer
	Tunnel Dialer

	once       sync.Once
	transports map[Route]*http.Transport
}

func (s *Server) dialer(r Route) Dialer {
	if r == Tunnel {
		return s.Tunnel
	}
	return s.Direct
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		s.handleConnect(w, r)
		return
	}
	if !r.URL.IsAbs() {
		http.Error(w, "vvcore: this is a proxy, not a web server", http.StatusBadRequest)
		return
	}
	s.handlePlain(w, r)
}

func (s *Server) handleConnect(w http.ResponseWriter, r *http.Request) {
	route := s.Rules.Match(r.Host)
	upstream, err := s.dialer(route).DialContext(r.Context(), "tcp", r.Host)
	if err != nil {
		logDialError(err, "%-6s CONNECT %s", route, r.Host)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	log.Infof("%-6s CONNECT %s", route, r.Host)

	hj, ok := w.(http.Hijacker)
	if !ok {
		upstream.Close()
		http.Error(w, "hijacking unsupported", http.StatusInternalServerError)
		return
	}
	client, buf, err := hj.Hijack()
	if err != nil {
		upstream.Close()
		return
	}
	if _, err := client.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
		client.Close()
		upstream.Close()
		return
	}
	// Bytes the client sent right after the CONNECT line may already sit in
	// the server's read buffer.
	if n := buf.Reader.Buffered(); n > 0 {
		pending, _ := buf.Reader.Peek(n)
		if _, err := upstream.Write(pending); err != nil {
			client.Close()
			upstream.Close()
			return
		}
	}
	pipe(client, upstream)
}

// logDialError reports a failed upstream request. Browsers routinely cancel
// speculative connections, so cancellations are only logged at debug level.
func logDialError(err error, format string, args ...any) {
	if errors.Is(err, context.Canceled) {
		log.WithError(err).Debugf(format, args...)
		return
	}
	log.WithError(err).Warnf(format, args...)
}

// pipe copies both ways until either side finishes, then closes both.
func pipe(a, b net.Conn) {
	done := make(chan struct{}, 2)
	cp := func(dst, src net.Conn) {
		io.Copy(dst, src)
		done <- struct{}{}
	}
	go cp(a, b)
	go cp(b, a)
	<-done
	a.Close()
	b.Close()
	<-done
}

var hopHeaders = []string{
	"Connection", "Proxy-Connection", "Keep-Alive", "Proxy-Authenticate",
	"Proxy-Authorization", "Te", "Trailer", "Transfer-Encoding", "Upgrade",
}

func (s *Server) handlePlain(w http.ResponseWriter, r *http.Request) {
	s.once.Do(func() {
		s.transports = map[Route]*http.Transport{
			Direct: {DialContext: s.Direct.DialContext},
			Tunnel: {DialContext: s.Tunnel.DialContext},
		}
	})
	route := s.Rules.Match(r.URL.Host)

	out := r.Clone(r.Context())
	out.RequestURI = ""
	for _, h := range hopHeaders {
		out.Header.Del(h)
	}
	resp, err := s.transports[route].RoundTrip(out)
	if err != nil {
		logDialError(err, "%-6s %s %s", route, r.Method, r.URL.Host)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	log.Infof("%-6s %s %s", route, r.Method, r.URL.Host)

	for _, h := range hopHeaders {
		resp.Header.Del(h)
	}
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
