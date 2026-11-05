package mountcontract

import "sync"

// MonotonicGeneration rejects generation rollback within one process lifetime.
// It is not a durable authority and must not be used as a replacement for one.
type MonotonicGeneration struct {
	mu       sync.Mutex
	source   AuthorizationGeneration
	highest  uint64
	poisoned bool
}

func NewMonotonicGeneration(source AuthorizationGeneration) *MonotonicGeneration {
	return &MonotonicGeneration{source: source}
}

// Check returns a nonzero generation only when it has never decreased.
// A rollback, missing generation, source error, or source panic permanently
// poisons this instance. MaxUint64 is valid and is not a poison sentinel.
func (m *MonotonicGeneration) Check() (generation uint64, checkErr error) {
	if m == nil || m.source == nil {
		return 0, ErrReadNotAuthorized
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	// A panic in the untrusted authority callback must never permit a later
	// successful check through this same guard.
	defer func() {
		if recover() != nil {
			m.poisoned = true
			generation = 0
			checkErr = ErrReadNotAuthorized
		}
	}()
	if m.poisoned {
		return 0, ErrReadNotAuthorized
	}
	value, err := m.source()
	if err != nil || value == 0 || value < m.highest {
		m.poisoned = true
		return 0, ErrReadNotAuthorized
	}
	m.highest = value
	return value, nil
}
