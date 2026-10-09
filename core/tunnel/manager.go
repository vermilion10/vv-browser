package tunnel

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/apex/log"
)

// Candidate is a server the Manager may connect to.
type Candidate struct {
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

	mu       sync.Mutex
	cur      *Tunnel
	lastGood string
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
	if err != nil {
		return nil, fmt.Errorf("tunnel: listing servers: %w", err)
	}
	if len(cands) == 0 {
		return nil, errors.New("tunnel: no servers to try")
	}
	cands = m.preferLastGood(cands)
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
		m.cur, m.lastGood = t, c.Name
		return t, nil
	}
	return nil, fmt.Errorf("tunnel: all servers failed: %w", errors.Join(errs...))
}

func (m *Manager) preferLastGood(cands []Candidate) []Candidate {
	for i, c := range cands {
		if c.Name == m.lastGood && i > 0 {
			out := append([]Candidate{c}, cands[:i]...)
			return append(out, cands[i+1:]...)
		}
	}
	return cands
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
