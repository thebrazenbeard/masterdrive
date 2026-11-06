// Package grantstate implements a fail-closed, in-memory source-side grant
// decision model. It is NOT persistent authority or an authenticated pairing protocol.
package grantstate

import (
 "errors"
 "sync"
 "time"
)

var (
 ErrDenied = errors.New("grantstate: denied")
 ErrInvalid = errors.New("grantstate: invalid transition")
 ErrUnavailable = errors.New("grantstate: authority unavailable")
)

type State uint8
const (
 Pending State = iota + 1
 Active
 Revoked
)
type Grant struct {
 ID string
 ConsumerKey string
 ShareID string
 ExpiresAt time.Time
 State State
 Epoch uint64
}
type Store struct {
 mu sync.Mutex
 epoch uint64
 available bool
 grants map[string]Grant
 now func() time.Time
}
func New(now func() time.Time) *Store {
 if now == nil { now = time.Now }
 return &Store{available:true, grants:make(map[string]Grant), now:now}
}
func (s *Store) SetAvailable(available bool) {
 s.mu.Lock(); defer s.mu.Unlock()
 s.available=available
}
// Propose creates a pending record; it does not authorize any read.
// ConsumerKey is a caller-supplied fingerprint and is NOT verified by this package.
func (s *Store) Propose(id, consumerKey, shareID string, expiry time.Time) error {
 s.mu.Lock(); defer s.mu.Unlock()
 if !s.available { return ErrUnavailable }
 if id=="" || consumerKey=="" || shareID=="" || !expiry.After(s.now()) { return ErrInvalid }
 if _, exists:=s.grants[id]; exists { return ErrInvalid }
 s.grants[id]=Grant{ID:id, ConsumerKey:consumerKey, ShareID:shareID, ExpiresAt:expiry, State:Pending}
 return nil
}
// Approve must only be called following separately verified owner consent.
func (s *Store) Approve(id string) (uint64,error) {
 s.mu.Lock(); defer s.mu.Unlock()
 if !s.available { return 0,ErrUnavailable }
 g,ok:=s.grants[id]
 if !ok || g.State!=Pending || !g.ExpiresAt.After(s.now()) || s.epoch==^uint64(0) { return 0,ErrInvalid }
 s.epoch++
 g.State=Active; g.Epoch=s.epoch; s.grants[id]=g
 return s.epoch,nil
}
// Revoke denies an existing grant in this process before returning.
// It does not commit to durable storage, so must NOT acknowledge durable revocation.
func (s *Store) Revoke(id string) (uint64,error) {
 s.mu.Lock(); defer s.mu.Unlock()
 if !s.available { return 0,ErrUnavailable }
 g,ok:=s.grants[id]
 if !ok || g.State!=Active || s.epoch==^uint64(0) { return 0,ErrInvalid }
 s.epoch++
 g.State=Revoked; g.Epoch=s.epoch; s.grants[id]=g
 return s.epoch,nil
}
// Authorize checks exact grant, consumer fingerprint, share, and observed epoch.
// This is a decision seam only; a caller MUST authenticate the peer independently,
// check pinned file versions and repeat checks at the source before each range.
func (s *Store) Authorize(id, consumerKey, shareID string, observedEpoch uint64) error {
 s.mu.Lock(); defer s.mu.Unlock()
 if !s.available { return ErrUnavailable }
 g,ok:=s.grants[id]
 if !ok || g.State!=Active || g.ConsumerKey!=consumerKey || g.ShareID!=shareID ||
  !g.ExpiresAt.After(s.now()) || observedEpoch!=s.epoch || g.Epoch>observedEpoch { return ErrDenied }
 return nil
}
