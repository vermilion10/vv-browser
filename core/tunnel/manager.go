package tunnel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/apex/log"
)

// Candidate is a server the Manager may connect to.
type Candidate struct {
	// ID identifies the server across server-list refreshes.
	ID string
	// Name labels the server in logs.
	Name   string
	Config []byte
}

// Manager keeps one tunnel up on demand, reconnecting (and falling back to
// other candidates) whenever the current one drops.
type Manager struct {
	// Candidates returns servers to try, in order of preference.
	Candidates func(ctx context.Context) ([]Candidate, error)
	// MTU of the userspace interface.
	MTU int
	// AttemptTimeout bounds each connection attempt.
	AttemptTimeout time.Duration
	// MaxAttempts caps how many candidates one connect cycle tries.
	MaxAttempts int
	// StatePath, if set, is a file remembering the last server that
	// worked. It is tried first on later connects, even after it drops out
	// of the server list, so the exit IP seen by websites stays the same
	// across launches.
	StatePath string

	mu  sync.Mutex
	cur *Tunnel
}

// Get returns the live tunnel, connecting first if there is none.
func (m *Manager) Get(ctx context.Context) (*Tunnel, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cur != nil {
		select {
		case <-m.cur.Done():
			m.cur = nil
		default:
			return m.cur, nil
		}
	}

	cands, err := m.Candidates(ctx)
	preferred, havePreferred := m.loadPreferred()
	if err != nil {
		if !havePreferred {
			return nil, fmt.Errorf("tunnel: listing servers: %w", err)
		}
		log.WithError(err).Warn("tunnel: cannot list servers, trying the saved one only")
	}
	if havePreferred {
		// VPN Gate relays sometimes reject a login and accept the next one;
		// a second try is cheaper than a new exit IP.
		cands = putFirst(cands, preferred)
		cands = append([]Candidate{cands[0]}, cands...)
	}
	if len(cands) == 0 {
		return nil, errors.New("tunnel: no servers to try")
	}
	if m.MaxAttempts > 0 && len(cands) > m.MaxAttempts {
		cands = cands[:m.MaxAttempts]
	}

	var errs []error
	for _, c := range cands {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		log.Infof("tunnel: connecting to %s", c.Name)
		actx, cancel := context.WithTimeout(ctx, m.AttemptTimeout)
		t, err := Start(actx, c.Config, m.MTU)
		cancel()
		if err != nil {
			log.WithError(err).Warnf("tunnel: %s failed", c.Name)
			errs = append(errs, fmt.Errorf("%s: %w", c.Name, err))
			continue
		}
		log.Infof("tunnel: up via %s (local %s)", c.Name, t.LocalIP())
		m.cur = t
		m.savePreferred(c)
		return t, nil
	}
	return nil, fmt.Errorf("tunnel: all servers failed: %w", errors.Join(errs...))
}

// putFirst moves the candidate with p's ID to the front, or prepends p if
// the list no longer has it.
func putFirst(cands []Candidate, p Candidate) []Candidate {
	out := []Candidate{p}
	for _, c := range cands {
		if c.ID == p.ID {
			out[0] = c // fresher config and stats
			continue
		}
		out = append(out, c)
	}
	return out
}

func (m *Manager) loadPreferred() (Candidate, bool) {
	if m.StatePath == "" {
		return Candidate{}, false
	}
	b, err := os.ReadFile(m.StatePath)
	if err != nil {
		return Candidate{}, false
	}
	var c Candidate
	if err := json.Unmarshal(b, &c); err != nil || c.ID == "" || len(c.Config) == 0 {
		return Candidate{}, false
	}
	return c, true
}

func (m *Manager) savePreferred(c Candidate) {
	if m.StatePath == "" || c.ID == "" {
		return
	}
	b, err := json.Marshal(c)
	if err == nil {
		err = os.MkdirAll(filepath.Dir(m.StatePath), 0o700)
	}
	if err == nil {
		err = os.WriteFile(m.StatePath, b, 0o600)
	}
	if err != nil {
		log.WithError(err).Warn("tunnel: cannot save the preferred server")
	}
}

// ForgetPreferred drops the saved server, so the next connect starts from
// the top of the server list.
func (m *Manager) ForgetPreferred() {
	if m.StatePath != "" {
		os.Remove(m.StatePath)
	}
}

// DialContext dials through the current tunnel, bringing one up if needed.
func (m *Manager) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	t, err := m.Get(ctx)
	if err != nil {
		return nil, err
	}
	return t.DialContext(ctx, network, address)
}

// Close shuts the current tunnel down.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cur != nil {
		m.cur.Close()
		m.cur = nil
	}
	return nil
}
