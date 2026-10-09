// Package mobile is the gomobile-facing API of the network core. Only types
// gomobile can bind (strings, ints, errors, interfaces) cross this boundary.
package mobile

import (
	"context"
	"errors"
	"sync"

	"github.com/apex/log"

	"vvbrowser/core/engine"
)

// Logger receives the core's log lines.
type Logger interface {
	Log(level string, message string)
}

type logHandler struct{ l Logger }

func (h logHandler) HandleLog(e *log.Entry) error {
	msg := e.Message
	if err, ok := e.Fields["error"]; ok {
		msg += ": " + errString(err)
	}
	h.l.Log(e.Level.String(), msg)
	return nil
}

func errString(v any) string {
	if err, ok := v.(error); ok {
		return err.Error()
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// SetLogger routes log output to l. Call it before Start.
func SetLogger(l Logger) {
	log.SetHandler(logHandler{l})
	log.SetLevel(log.InfoLevel)
}

var (
	mu      sync.Mutex
	current *engine.Engine
	addr    string
)

// Start begins listening on a free loopback port and returns its
// "host:port". The tunnel is brought up by Connect; until then, requests
// routed through it wait for the handshake. Calling Start again returns the
// existing address.
func Start() (string, error) {
	mu.Lock()
	defer mu.Unlock()
	if current != nil {
		return addr, nil
	}
	eng := engine.New(engine.Options{})
	a, err := eng.Listen()
	if err != nil {
		return "", err
	}
	current, addr = eng, a
	return a, nil
}

// Connect brings the tunnel up, blocking until it is ready or every
// candidate relay has failed.
func Connect() error {
	eng := get()
	if eng == nil {
		return errors.New("mobile: not started")
	}
	return eng.Connect(context.Background())
}

// ExitInfo returns ipinfo.io's JSON for the tunnel's exit address.
func ExitInfo() (string, error) {
	eng := get()
	if eng == nil {
		return "", errors.New("mobile: not started")
	}
	return eng.ExitInfo(context.Background())
}

// Stop shuts the proxy and tunnel down.
func Stop() {
	mu.Lock()
	defer mu.Unlock()
	if current != nil {
		current.Close()
		current, addr = nil, ""
	}
}

func get() *engine.Engine {
	mu.Lock()
	defer mu.Unlock()
	return current
}
