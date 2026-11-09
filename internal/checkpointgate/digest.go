package checkpointgate

import (
 "crypto/sha256"
 "encoding/binary"
 "errors"
 "sort"
)

var ErrInvalidLedger = errors.New("checkpointgate: invalid canonical ledger")

// Revocation is an opaque, source-owned tombstone identity. It must represent
// a durable revoked credential/grant, never a caller-supplied active grant.
type Revocation struct {
 ID string
 Epoch uint64
}

// DigestRevocations computes a domain-separated digest over the complete set
// of revocation tombstones. Entries are sorted without mutating caller input.
// This is not an authenticated checkpoint or a durable monotonic ledger.
func DigestRevocations(storeID string, revision uint64, entries []Revocation) ([32]byte, error) {
 var zero [32]byte
 if storeID == "" || revision == 0 || len(storeID) > 65535 || len(entries) > 1000000 {
  return zero, ErrInvalidLedger
 }
 copyEntries := append([]Revocation(nil), entries...)
 sort.Slice(copyEntries, func(i,j int) bool { return copyEntries[i].ID < copyEntries[j].ID })
 for i,e := range copyEntries {
  if e.ID == "" || len(e.ID) > 65535 || e.Epoch == 0 || (i>0 && copyEntries[i-1].ID == e.ID) {
   return zero, ErrInvalidLedger
  }
 }
 h := sha256.New()
 h.Write([]byte("masterdrive/revocation-ledger/v1\x00"))
 var b [8]byte
 binary.BigEndian.PutUint64(b[:], uint64(len(storeID))); h.Write(b[:]); h.Write([]byte(storeID))
 binary.BigEndian.PutUint64(b[:], revision); h.Write(b[:])
 binary.BigEndian.PutUint64(b[:], uint64(len(copyEntries))); h.Write(b[:])
 for _,e := range copyEntries {
  binary.BigEndian.PutUint64(b[:],uint64(len(e.ID))); h.Write(b[:]); h.Write([]byte(e.ID))
  binary.BigEndian.PutUint64(b[:],e.Epoch); h.Write(b[:])
 }
 var out [32]byte
 copy(out[:],h.Sum(nil))
 return out,nil
}
