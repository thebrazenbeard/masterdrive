package mountcontract

import "sync"

// MonotonicGeneration rejects generation rollback within one process lifetime.
// It is not a durable authority and must not be used as a replacement for one.
type MonotonicGeneration struct {
	mu      sync.Mutex
	source  AuthorizationGeneration
	highest uint64
}

func NewMonotonicGeneration(source AuthorizationGeneration) *MonotonicGeneration {
	return &MonotonicGeneration{source: source}
}

// Check returns a generation only when it is nonzero and has never decreased.
// After rollback detection the instance stays failed closed, even if the source recovers.
func (m *MonotonicGeneration) Check() (uint64, error) {
	if m == nil || m.source == nil {
		return 0, ErrReadNotAuthorized
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.highest == ^uint64(0) {
		return 0, ErrReadNotAuthorized
	}
	value, err := m.source()
	if err != nil || value == 0 || value < m.highest {
		m.highest = ^uint64(0)
		return 0, ErrReadNotAuthorized
	}
	m.highest = value
	return value, nil
}
