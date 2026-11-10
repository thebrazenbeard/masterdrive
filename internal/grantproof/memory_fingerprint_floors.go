package grantproof

import (
 "crypto/subtle"
 "sync"
)

// MemoryFingerprintFloors is an isolated, concurrency-safe reference store.
// It is deliberately NOT durable and MUST NOT authorize real reads or mounts.
// Provisioning is explicit; unknown relationships fail closed.
type MemoryFingerprintFloors struct {
 mu sync.Mutex
 floors map[relationshipKey]fingerprintRecord
}
type relationshipKey struct { source, audience, share string }
type fingerprintRecord struct { revision uint64; fingerprint [32]byte }

func relationship(source, audience, share string) (relationshipKey, bool) {
 return relationshipKey{source, audience, share}, source != "" && audience != "" && share != ""
}

// Provision initializes a previously unknown relationship. Existing records
// cannot be overwritten or reset, even by a caller claiming to reprovision.
func (m *MemoryFingerprintFloors) Provision(source, audience, share string, revision uint64, fingerprint [32]byte) error {
 key, valid := relationship(source, audience, share)
 if m == nil || !valid || revision == 0 { return ErrFloorUnavailable }
 m.mu.Lock()
 defer m.mu.Unlock()
 if m.floors == nil { m.floors = make(map[relationshipKey]fingerprintRecord) }
 if _, exists := m.floors[key]; exists { return ErrFloorUnavailable }
 m.floors[key] = fingerprintRecord{revision, fingerprint}
 return nil
}

func (m *MemoryFingerprintFloors) FingerprintFloor(source, audience, share string) (uint64, [32]byte, error) {
 key, valid := relationship(source, audience, share)
 if m == nil || !valid { return 0, [32]byte{}, ErrFloorUnavailable }
 m.mu.Lock()
 defer m.mu.Unlock()
 record, exists := m.floors[key]
 if !exists || record.revision == 0 { return 0, [32]byte{}, ErrFloorUnavailable }
 return record.revision, record.fingerprint, nil
}

func (m *MemoryFingerprintFloors) CommitFingerprintIfCurrent(source, audience, share string, revision uint64, fingerprint [32]byte) error {
 key, valid := relationship(source, audience, share)
 if m == nil || !valid || revision == 0 { return ErrFloorUnavailable }
 m.mu.Lock()
 defer m.mu.Unlock()
 prior, exists := m.floors[key]
 if !exists || prior.revision == 0 || revision < prior.revision { return ErrFloorUnavailable }
 if revision == prior.revision && subtle.ConstantTimeCompare(prior.fingerprint[:], fingerprint[:]) != 1 {
  return ErrCheckpointEquivocation
 }
 m.floors[key] = fingerprintRecord{revision, fingerprint}
 return nil
}
