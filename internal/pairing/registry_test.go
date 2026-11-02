package pairing

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"
)

func keypair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return pub, priv
}

func redeem(t *testing.T, r *Registry, pub ed25519.PublicKey, priv ed25519.PrivateKey, shares ...string) string {
	t.Helper()
	secret, err := r.IssueLocalInvite(pub, shares, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	id, err := r.RedeemInvite(secret, ed25519.Sign(priv, InviteProofBytes(secret)))
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func confirmed(t *testing.T, r *Registry, pub ed25519.PublicKey, priv ed25519.PrivateKey, shares ...string) string {
	t.Helper()
	id := redeem(t, r, pub, priv, shares...)
	fp, err := Fingerprint(pub)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ConfirmLocal(id, fp); err != nil {
		t.Fatal(err)
	}
	return id
}

func proof(t *testing.T, id string, priv ed25519.PrivateKey, at time.Time, share, op, path string) Proof {
	t.Helper()
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	p := Proof{PairID: id, Epoch: 1, Operation: op, ShareID: share, Path: path, At: at, Nonce: nonce}
	p.Signature = ed25519.Sign(priv, p.SigningBytes())
	return p
}

func denied(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, ErrDenied) {
		t.Fatalf("expected ErrDenied, got %v", err)
	}
}

func TestInviteCannotSelfEnrollAndIsOneUse(t *testing.T) {
	now := time.Date(2026, time.November, 2, 0, 0, 0, 0, time.UTC)
	r := newRegistry(func() time.Time { return now })
	pub, priv := keypair(t)
	_, unrelatedPrivate := keypair(t)
	if _, err := r.IssueLocalInvite(pub, []string{"documents"}, MaxInviteLifetime+time.Second); !errors.Is(err, ErrInvalid) {
		t.Fatalf("lifetime bound: %v", err)
	}
	if _, err := r.IssueLocalInvite(pub, []string{"documents", "documents"}, time.Minute); !errors.Is(err, ErrInvalid) {
		t.Fatalf("duplicate grant bound: %v", err)
	}
	secret, err := r.IssueLocalInvite(pub, []string{"documents"}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	denied(t, func() error {
		_, err := r.RedeemInvite(secret, ed25519.Sign(unrelatedPrivate, InviteProofBytes(secret)))
		return err
	}())
	validSignature := ed25519.Sign(priv, InviteProofBytes(secret))
	id, err := r.RedeemInvite(secret, validSignature)
	if err != nil || id == "" {
		t.Fatalf("valid key-bound redemption: %v", err)
	}
	denied(t, func() error {
		_, err := r.RedeemInvite(secret, validSignature)
		return err
	}())

	otherPub, otherPriv := keypair(t)
	secret, err = r.IssueLocalInvite(otherPub, []string{"documents"}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute)
	denied(t, func() error {
		_, err := r.RedeemInvite(secret, ed25519.Sign(otherPriv, InviteProofBytes(secret)))
		return err
	}())
}

func TestPendingConfirmationExpiresAndAllowsCleanReissue(t *testing.T) {
	clock := time.Date(2026, time.November, 2, 0, 0, 0, 0, time.UTC)
	r := newRegistry(func() time.Time { return clock })
	pub, priv := keypair(t)
	oldID := redeem(t, r, pub, priv, "a")
	fp, err := Fingerprint(pub)
	if err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(MaxPendingConfirmLifetime)
	denied(t, r.ConfirmLocal(oldID, fp))
	denied(t, func() error {
		_, err := r.PendingFingerprintLocal(oldID)
		return err
	}())
	// An expired, never-approved pair is not a permanent key tombstone.
	newID := redeem(t, r, pub, priv, "b")
	if newID == oldID {
		t.Fatal("new pair reused expired pair ID")
	}
	if err := r.ConfirmLocal(newID, fp); err != nil {
		t.Fatal(err)
	}
	denied(t, r.Authorize(proof(t, oldID, priv, clock, "a", "list", "")))
}

func TestControllerKeyUniqueThroughInvitePendingConfirmedAndRevoked(t *testing.T) {
	r := NewRegistry()
	pub, priv := keypair(t)
	secret, err := r.IssueLocalInvite(pub, []string{"a", "b"}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	denied(t, func() error {
		_, err := r.IssueLocalInvite(pub, []string{"c"}, time.Minute)
		return err
	}())
	id, err := r.RedeemInvite(secret, ed25519.Sign(priv, InviteProofBytes(secret)))
	if err != nil {
		t.Fatal(err)
	}
	denied(t, func() error {
		_, err := r.IssueLocalInvite(pub, []string{"c"}, time.Minute)
		return err
	}())
	fp, _ := Fingerprint(pub)
	if err := r.ConfirmLocal(id, fp); err != nil {
		t.Fatal(err)
	}
	denied(t, func() error {
		_, err := r.IssueLocalInvite(pub, []string{"c"}, time.Minute)
		return err
	}())
	if err := r.RevokeLocal(id); err != nil {
		t.Fatal(err)
	}
	denied(t, func() error {
		_, err := r.IssueLocalInvite(pub, []string{"c"}, time.Minute)
		return err
	}())
	// A rotated public key is a new identity and may be owner-invited.
	replacementPub, _ := keypair(t)
	if _, err := r.IssueLocalInvite(replacementPub, []string{"c"}, time.Minute); err != nil {
		t.Fatal(err)
	}
}

func TestPendingPairCapAndExpiryGarbageCollection(t *testing.T) {
	clock := time.Date(2026, time.November, 2, 0, 0, 0, 0, time.UTC)
	r := newRegistry(func() time.Time { return clock })
	var oldest string
	for i := 0; i < maxPendingPairs; i++ {
		pub, priv := keypair(t)
		id := redeem(t, r, pub, priv, "a")
		if i == 0 {
			oldest = id
		}
	}
	pub, priv := keypair(t)
	if _, err := r.IssueLocalInvite(pub, []string{"a"}, time.Minute); !errors.Is(err, ErrBusy) {
		t.Fatalf("expected pending-pair capacity limit, got %v", err)
	}
	clock = clock.Add(MaxPendingConfirmLifetime)
	newID := redeem(t, r, pub, priv, "a")
	if newID == "" {
		t.Fatal("new pair missing after expired-pending cleanup")
	}
	denied(t, func() error {
		_, err := r.PendingFingerprintLocal(oldest)
		return err
	}())
}

func TestTotalPairRecordCapIncludesRevokedTombstones(t *testing.T) {
	clock := time.Date(2026, time.November, 2, 0, 0, 0, 0, time.UTC)
	r := newRegistry(func() time.Time { return clock })
	// White-box fixture isolates the total-cap gate without generating and
	// enrolling 1,024 actual controllers. All records are non-expiring
	// revocation tombstones, so lazy pending GC must not reclaim them.
	r.mu.Lock()
	for i := 0; i < maxPairRecords; i++ {
		r.pairs["revoked-"+strconv.Itoa(i)] = &pair{revoked: true}
	}
	r.mu.Unlock()
	pub, _ := keypair(t)
	if _, err := r.IssueLocalInvite(pub, []string{"a"}, time.Minute); !errors.Is(err, ErrBusy) {
		t.Fatalf("expected total record cap to deny new invite, got %v", err)
	}
	clock = clock.Add(24 * time.Hour)
	if _, err := r.IssueLocalInvite(pub, []string{"a"}, time.Minute); !errors.Is(err, ErrBusy) {
		t.Fatalf("revoked records were improperly garbage collected: %v", err)
	}
}

func TestSignedSearchRequestIsExplicitlyUnsupported(t *testing.T) {
	now := time.Date(2026, time.November, 2, 0, 0, 0, 0, time.UTC)
	r := newRegistry(func() time.Time { return now })
	pub, priv := keypair(t)
	id := confirmed(t, r, pub, priv, "a")
	// The signature is valid: rejection must come from unsupported intent,
	// not from signature tampering. A future protocol must bind query terms.
	signedSearch := proof(t, id, priv, now, "a", "search", "")
	denied(t, r.Authorize(signedSearch))
	denied(t, func() error {
		_, _, err := r.AuthorizeStream(context.Background(), signedSearch)
		return err
	}())
}

func TestPendingRequiresIndependentLocalConfirmation(t *testing.T) {
	now := time.Now()
	r := newRegistry(func() time.Time { return now })
	pub, priv := keypair(t)
	id := redeem(t, r, pub, priv, "home")
	request := proof(t, id, priv, now, "home", "download", "my-file.txt")
	denied(t, r.Authorize(request))
	denied(t, r.ConfirmLocal(id, "not-the-controller-fingerprint"))
	fp, err := r.PendingFingerprintLocal(id)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := Fingerprint(pub)
	if err != nil || fp != expected {
		t.Fatal("local fingerprint mismatched controller public key")
	}
	if err := r.ConfirmLocal(id, expected); err != nil {
		t.Fatal(err)
	}
	if err := r.Authorize(request); err != nil {
		t.Fatal(err)
	}
	denied(t, r.ConfirmLocal(id, expected))
}

func TestReadProofScopeTamperingReplayAndRevocation(t *testing.T) {
	now := time.Date(2026, time.November, 2, 0, 0, 0, 0, time.UTC)
	r := newRegistry(func() time.Time { return now })
	pub, priv := keypair(t)
	id := confirmed(t, r, pub, priv, "photos")
	good := proof(t, id, priv, now, "photos", "download", "photo.jpg")
	if err := r.Authorize(good); err != nil {
		t.Fatal(err)
	}
	denied(t, r.Authorize(good))

	for _, mutate := range []func(*Proof){
		func(p *Proof) { p.ShareID = "private" },
		func(p *Proof) { p.Path = "other.jpg" },
		func(p *Proof) { p.Operation = "search" },
		func(p *Proof) { p.Path = "../escape" },
		func(p *Proof) { p.Path = `C:\\windows` },
		func(p *Proof) { p.Path = "dir\\file" },
		func(p *Proof) { p.Path = "." },
		func(p *Proof) { p.Signature = make([]byte, ed25519.SignatureSize) },
		func(p *Proof) { p.Epoch++ },
	} {
		p := proof(t, id, priv, now, "photos", "download", "photo.jpg")
		mutate(&p)
		denied(t, r.Authorize(p))
	}

	tooOld := proof(t, id, priv, now.Add(-MaxProofSkew-time.Second), "photos", "list", "")
	denied(t, r.Authorize(tooOld))
	tooNew := proof(t, id, priv, now.Add(MaxProofSkew+time.Second), "photos", "list", "")
	denied(t, r.Authorize(tooNew))

	if err := r.RevokeLocal(id); err != nil {
		t.Fatal(err)
	}
	denied(t, r.Authorize(proof(t, id, priv, now, "photos", "list", "")))
	denied(t, r.RevokeLocal(id))
	denied(t, NewRegistry().Authorize(proof(t, id, priv, now, "photos", "list", "")))
}

func TestFutureDatedProofCannotReplayAfterThirtySeconds(t *testing.T) {
	clock := time.Date(2026, time.November, 2, 0, 0, 0, 0, time.UTC)
	r := newRegistry(func() time.Time { return clock })
	pub, priv := keypair(t)
	id := confirmed(t, r, pub, priv, "a")
	p := proof(t, id, priv, clock.Add(25*time.Second), "a", "list", "")
	if err := r.Authorize(p); err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(35 * time.Second)
	// Signature is still timestamp-valid for 20 more seconds. Replay
	// bookkeeping must survive longer than now+MaxProofSkew.
	denied(t, r.Authorize(p))
}

func TestRevocationSignalsActiveStreamLease(t *testing.T) {
	now := time.Date(2026, time.November, 2, 0, 0, 0, 0, time.UTC)
	r := newRegistry(func() time.Time { return now })
	pub, priv := keypair(t)
	id := confirmed(t, r, pub, priv, "backups")
	streamProof := proof(t, id, priv, now, "backups", "download", "archive.bin")
	ctx, release, err := r.AuthorizeStream(context.Background(), streamProof)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if ctx.Err() != nil {
		t.Fatal("fresh authorized stream unexpectedly canceled")
	}
	if err := r.RevokeLocal(id); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("revocation did not signal active stream")
	}
	denied(t, func() error {
		_, _, err := r.AuthorizeStream(context.Background(), proof(t, id, priv, now, "backups", "download", "archive.bin"))
		return err
	}())
	release()
	release() // idempotent
}

func TestInviteConcurrentRedemptionHasSingleWinner(t *testing.T) {
	r := NewRegistry()
	pub, priv := keypair(t)
	secret, err := r.IssueLocalInvite(pub, []string{"a"}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	sig := ed25519.Sign(priv, InviteProofBytes(secret))
	var wg sync.WaitGroup
	results := make(chan error, 24)
	for i := 0; i < cap(results); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := r.RedeemInvite(secret, sig)
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else if !errors.Is(err, ErrDenied) {
			t.Fatalf("unexpected concurrent error: %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("expected exactly one redemption, got %d", successes)
	}
}
