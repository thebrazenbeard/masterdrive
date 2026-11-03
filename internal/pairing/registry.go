// Package pairing is an isolated, in-memory prototype of owner-approved
// MasterDrive controller pairing. It is not a network enrollment endpoint,
// credential vault, Tailscale identity adapter, or persistent pair database.
//
// Only trusted LOCAL operator code may call IssueLocalInvite, ConfirmLocal,
// or RevokeLocal. Those methods must never be exposed as anonymous HTTP APIs.
package pairing

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io/fs"
	"strings"
	"sync"
	"time"
)

const (
	MaxInviteLifetime = 5 * time.Minute
	MaxPendingConfirmLifetime = 5 * time.Minute
	MaxProofSkew = 30 * time.Second
	MaxGrantsPerPair = 64
	maxPendingInvites = 128
	maxPendingPairs = 128
	maxPairRecords = 1024
	maxSeenNoncesPerPair = 1024
)

var (
	ErrInvalid = errors.New("pairing: invalid request")
	ErrDenied = errors.New("pairing: not authorized")
	ErrBusy = errors.New("pairing: capacity exhausted")
)

type invite struct {
	controllerKey ed25519.PublicKey
	shares map[string]struct{}
	expires time.Time
}

type pair struct {
	controllerKey ed25519.PublicKey
	shares map[string]struct{}
	fingerprint string
	pendingExpires time.Time
	epoch uint64
	confirmed bool
	revoked bool
	seen map[[16]byte]time.Time
	active map[uint64]context.CancelFunc
	nextLease uint64
}

// Registry is intentionally ephemeral: a process restart forgets all pairs
// and therefore denies access. Persistent, crash-safe storage is a separate
// security and platform qualification milestone.
type Registry struct {
	mu sync.Mutex
	now func() time.Time
	invites map[[sha256.Size]byte]invite
	pairs map[string]*pair
}

func NewRegistry() *Registry {
	return newRegistry(time.Now)
}

func newRegistry(now func() time.Time) *Registry {
	return &Registry{
		now: now,
		invites: make(map[[sha256.Size]byte]invite),
		pairs: make(map[string]*pair),
	}
}

// purgeExpiredLocked removes only unredeemed invitations and unconfirmed
// pairs that have passed their local-confirmation deadline. Revoked records
// are retained as key tombstones: the same controller key cannot silently
// receive a fresh pair after its prior identity was revoked.
func (r *Registry) purgeExpiredLocked(now time.Time) {
	for digest, pending := range r.invites {
		if !now.Before(pending.expires) {
			delete(r.invites, digest)
		}
	}
	for id, p := range r.pairs {
		if !p.confirmed && !p.revoked && !now.Before(p.pendingExpires) {
			delete(r.pairs, id)
		}
	}
}

func (r *Registry) pendingCountLocked() int {
	count := 0
	for _, p := range r.pairs {
		if !p.confirmed && !p.revoked {
			count++
		}
	}
	return count
}

// A key may own one pending or confirmed pair only. Revoked key tombstones
// deliberately block silent key reuse until a new owner-authorized key is
// provisioned. Share permissions must be selected together at enrollment.
func (r *Registry) keyAlreadyUsedLocked(key ed25519.PublicKey) bool {
	for _, pending := range r.invites {
		if bytes.Equal(pending.controllerKey, key) {
			return true
		}
	}
	for _, p := range r.pairs {
		if bytes.Equal(p.controllerKey, key) {
			return true
		}
	}
	return false
}

func validShareID(id string) bool {
	if len(id) < 1 || len(id) > 64 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' ||
			c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func copyShares(ids []string) (map[string]struct{}, error) {
	if len(ids) == 0 || len(ids) > MaxGrantsPerPair {
		return nil, ErrInvalid
	}
	shares := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if !validShareID(id) {
			return nil, ErrInvalid
		}
		if _, duplicate := shares[id]; duplicate {
			return nil, ErrInvalid
		}
		shares[id] = struct{}{}
	}
	return shares, nil
}

func fingerprint(publicKey ed25519.PublicKey) string {
	sum := sha256.Sum256(publicKey)
	return hex.EncodeToString(sum[:])
}

// Fingerprint is the full SHA-256 fingerprint of a controller public key.
// Both device owners must compare it through an independent trusted channel.
func Fingerprint(publicKey ed25519.PublicKey) (string, error) {
	if len(publicKey) != ed25519.PublicKeySize {
		return "", ErrInvalid
	}
	return fingerprint(publicKey), nil
}

// IssueLocalInvite MUST be invoked only following an actual, local owner
// decision selecting the exact public key and share IDs. The 256-bit secret
// is returned once and only its digest is retained. Delivery is out of band.
func (r *Registry) IssueLocalInvite(controllerKey ed25519.PublicKey, shareIDs []string, lifetime time.Duration) (string, error) {
	if len(controllerKey) != ed25519.PublicKeySize || lifetime <= 0 || lifetime > MaxInviteLifetime {
		return "", ErrInvalid
	}
	shares, err := copyShares(shareIDs)
	if err != nil {
		return "", err
	}
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return "", err
	}
	digest := sha256.Sum256(secret[:])
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	r.purgeExpiredLocked(now)
	if len(r.invites) >= maxPendingInvites || len(r.pairs) >= maxPairRecords ||
		r.pendingCountLocked() >= maxPendingPairs {
		return "", ErrBusy
	}
	if r.keyAlreadyUsedLocked(controllerKey) {
		return "", ErrDenied
	}
	if _, collision := r.invites[digest]; collision {
		return "", ErrBusy
	}
	r.invites[digest] = invite{
		controllerKey: append(ed25519.PublicKey(nil), controllerKey...),
		shares: shares, expires: now.Add(lifetime),
	}
	return base64.RawURLEncoding.EncodeToString(secret[:]), nil
}

// InviteProofBytes is the domain-separated message that the invited
// controller signs to demonstrate possession of its private key.
func InviteProofBytes(secret string) []byte {
	return append([]byte("masterdrive-pair-invite-v1\x00"), []byte(secret)...)
}

// RedeemInvite consumes a matching unexpired invitation exactly once and
// creates a PENDING pair. It grants NO read access until ConfirmLocal.
func (r *Registry) RedeemInvite(secret string, signature []byte) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(secret)
	if err != nil || len(raw) != 32 {
		return "", ErrDenied
	}
	digest := sha256.Sum256(raw)
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	r.purgeExpiredLocked(now)
	pending, ok := r.invites[digest]
	if !ok || !now.Before(pending.expires) {
		return "", ErrDenied
	}
	if !ed25519.Verify(pending.controllerKey, InviteProofBytes(secret), signature) {
		return "", ErrDenied
	}
	if len(r.pairs) >= maxPairRecords || r.pendingCountLocked() >= maxPendingPairs {
		return "", ErrBusy
	}
	var idBytes [16]byte
	if _, err := rand.Read(idBytes[:]); err != nil {
		return "", err
	}
	id := base64.RawURLEncoding.EncodeToString(idBytes[:])
	if _, collision := r.pairs[id]; collision {
		return "", ErrBusy
	}
	delete(r.invites, digest)
	r.pairs[id] = &pair{
		controllerKey: append(ed25519.PublicKey(nil), pending.controllerKey...),
		shares: pending.shares,
		fingerprint: fingerprint(pending.controllerKey),
		pendingExpires: now.Add(MaxPendingConfirmLifetime),
		epoch: 1,
		seen: make(map[[16]byte]time.Time),
		active: make(map[uint64]context.CancelFunc),
	}
	return id, nil
}

// PendingFingerprintLocal is for a trusted local confirmation screen only.
func (r *Registry) PendingFingerprintLocal(pairID string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.purgeExpiredLocked(r.now())
	p, ok := r.pairs[pairID]
	if !ok || p.revoked || p.confirmed {
		return "", ErrDenied
	}
	return p.fingerprint, nil
}

// ConfirmLocal is a TRUSTED LOCAL owner action. It requires matching the full
// out-of-band controller fingerprint; it MUST NOT be reachable through the
// remote controller data API or merely by possession of the invite.
func (r *Registry) ConfirmLocal(pairID, independentlyVerifiedFingerprint string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.purgeExpiredLocked(r.now())
	p, ok := r.pairs[pairID]
	if !ok || p.revoked || p.confirmed ||
		len(independentlyVerifiedFingerprint) != len(p.fingerprint) ||
		subtle.ConstantTimeCompare([]byte(p.fingerprint), []byte(independentlyVerifiedFingerprint)) != 1 {
		return ErrDenied
	}
	p.confirmed = true
	p.pendingExpires = time.Time{}
	return nil
}

// RevokeLocal fences all NEW authorizations and signals cancellation to
// registered stream leases. A handler MUST stop its stream on the returned
// context's cancellation; this method cannot forcibly abort socket writes.
// It does not alter Tailscale policy or persist across process restarts.
func (r *Registry) RevokeLocal(pairID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.purgeExpiredLocked(r.now())
	p, ok := r.pairs[pairID]
	if !ok || p.revoked {
		return ErrDenied
	}
	p.revoked = true
	p.epoch++
	p.seen = nil
	for _, cancel := range p.active {
		cancel()
	}
	p.active = nil
	return nil
}

// Proof is signed by the controller for one source request. Operation must
// be list or full-file download. Search terms, HTTP Range, conditional-range
// headers, and other response-changing query/header values are NOT signed:
// handlers MUST reject those request variants under this proof version.
// The caller still must verify trusted transport identity and enforce os.Root.
type Proof struct {
	PairID string
	Epoch uint64
	Operation string
	ShareID string
	Path string
	At time.Time
	Nonce [16]byte
	Signature []byte
}

func writeString(buf *bytes.Buffer, value string) {
	_ = binary.Write(buf, binary.BigEndian, uint32(len(value)))
	_, _ = buf.WriteString(value)
}

// SigningBytes is the canonical, length-prefixed, versioned Ed25519 message.
// It binds pair ID, permission epoch, operation, share and path, time and
// request nonce. No HTTP headers or raw URLs are implicitly trusted.
func (p Proof) SigningBytes() []byte {
	var buf bytes.Buffer
	buf.WriteString("masterdrive-read-request-v1\x00")
	writeString(&buf, p.PairID)
	_ = binary.Write(&buf, binary.BigEndian, p.Epoch)
	writeString(&buf, p.Operation)
	writeString(&buf, p.ShareID)
	writeString(&buf, p.Path)
	_ = binary.Write(&buf, binary.BigEndian, p.At.UTC().UnixMilli())
	_, _ = buf.Write(p.Nonce[:])
	return buf.Bytes()
}

func validProof(p Proof) bool {
	if p.PairID == "" || len(p.PairID) > 64 || p.Epoch == 0 || !validShareID(p.ShareID) ||
		(p.Operation != "list" && p.Operation != "download") ||
		len(p.Signature) != ed25519.SignatureSize || p.At.IsZero() || len(p.Path) > 4096 {
		return false
	}
	// The protocol accepts only portable, slash-delimited logical paths.
	// Colon forbids Windows drive/ADS syntax; os.Root is still mandatory.
	if p.Path != "" && (!fs.ValidPath(p.Path) || p.Path == "." || strings.ContainsAny(p.Path, ":\\\x00")) {
		return false
	}
	if p.Operation == "download" && p.Path == "" {
		return false
	}
	return p.Nonce != [16]byte{}
}

// Authorize verifies a key-bound, one-time read proof. It is NOT middleware:
// integration must call it before opening the share and must separately
// validate a pinned network identity. No remote pairing/revocation routes
// are installed by this library.
func (r *Registry) Authorize(proof Proof) error {
	if !validProof(proof) {
		return ErrDenied
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	p, exists := r.pairs[proof.PairID]
	return authorizeLocked(p, exists, proof, now)
}

func authorizeLocked(p *pair, exists bool, proof Proof, now time.Time) error {
	if !exists || !p.confirmed || p.revoked || proof.Epoch != p.epoch {
		return ErrDenied
	}
	if _, allowed := p.shares[proof.ShareID]; !allowed {
		return ErrDenied
	}
	skew := now.Sub(proof.At)
	if skew > MaxProofSkew || skew < -MaxProofSkew {
		return ErrDenied
	}
	for nonce, expires := range p.seen {
		if !now.Before(expires) {
			delete(p.seen, nonce)
		}
	}
	if _, replay := p.seen[proof.Nonce]; replay || len(p.seen) >= maxSeenNoncesPerPair {
		return ErrDenied
	}
	if !ed25519.Verify(p.controllerKey, proof.SigningBytes(), proof.Signature) {
		return ErrDenied
	}
	// Cache until the proof's last possible accepted instant, including
	// proofs whose signed timestamp is in the allowed future-skew window.
	// Expiring at now+MaxProofSkew would permit delayed replay.
	p.seen[proof.Nonce] = proof.At.Add(MaxProofSkew).Add(time.Nanosecond)
	return nil
}

// AuthorizeStream creates a revocable, cooperative lease. File-stream code
// MUST use the returned context to abort when the owner revokes this pair,
// and MUST call release once the stream finishes. A canceled context alone
// cannot guarantee prompt interruption of a blocked writer or file copy.
// Non-streaming Authorize calls do not register a cancellation lease.
func (r *Registry) AuthorizeStream(parent context.Context, proof Proof) (context.Context, func(), error) {
	if parent == nil || parent.Err() != nil || !validProof(proof) {
		return nil, nil, ErrDenied
	}
	r.mu.Lock()
	p, exists := r.pairs[proof.PairID]
	if err := authorizeLocked(p, exists, proof, r.now()); err != nil {
		r.mu.Unlock()
		return nil, nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	p.nextLease++
	lease := p.nextLease
	p.active[lease] = cancel
	r.mu.Unlock()
	var once sync.Once
	release := func() {
		once.Do(func() {
			r.mu.Lock()
			delete(p.active, lease)
			r.mu.Unlock()
			cancel()
		})
	}
	return ctx, release, nil
}
