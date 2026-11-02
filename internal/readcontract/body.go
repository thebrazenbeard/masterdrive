package readcontract

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
)

const MaxBufferedPartial int64 = 16 << 20

var (
	ErrUnsafeBody = errors.New("read contract: unsafe partial body")
	ErrRevoked    = errors.New("read contract: grant revoked")
)

// ReadBoundedPartial stages an exact, version-pinned response body privately.
// No bytes are returned on a failed read, deadline, or observed revocation.
// The caller must supply an independently enforced grant and an interruptible
// transport. This function does not authenticate a peer or authorize a mount.
func ReadBoundedPartial(ctx context.Context, w ExpectedRange, resp *http.Response, maxBytes int64, revoked <-chan struct{}) ([]byte, error) {
	if resp == nil || resp.Body == nil {
		return nil, ErrUnsafeBody
	}
	var closeOnce sync.Once
	closeBody := func() { closeOnce.Do(func() { _ = resp.Body.Close() }) }
	defer closeBody()
	if ctx == nil || revoked == nil || maxBytes <= 0 || w.Length <= 0 ||
		w.Length > maxBytes || w.Length > MaxBufferedPartial {
		return nil, ErrUnsafeBody
	}
	if _, ok := ctx.Deadline(); !ok {
		return nil, ErrUnsafeBody
	}
	if err := ValidatePartial(w, resp); err != nil {
		return nil, err
	}
	if err := bodyGateState(ctx, revoked); err != nil {
		return nil, err
	}

	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		select {
		case <-ctx.Done():
			closeBody()
		case <-revoked:
			closeBody()
		case <-done:
		}
	}()
	defer func() { close(done); <-stopped }()

	buf := make([]byte, int(w.Length))
	used, noProgress := 0, 0
	for used < len(buf) {
		if err := bodyGateState(ctx, revoked); err != nil {
			return nil, err
		}
		end := used + 32*1024
		if end > len(buf) {
			end = len(buf)
		}
		part := buf[used:end]
		n, readErr := resp.Body.Read(part)
		if err := bodyGateState(ctx, revoked); err != nil {
			return nil, err
		}
		if n < 0 || n > len(part) {
			return nil, ErrUnsafeBody
		}
		used += n
		if readErr != nil {
			if errors.Is(readErr, io.EOF) && used == len(buf) {
				if err := bodyGateState(ctx, revoked); err != nil {
					return nil, err
				}
				return buf, nil
			}
			return nil, ErrUnsafeBody
		}
		if n == 0 {
			noProgress++
			if noProgress >= 100 {
				return nil, ErrUnsafeBody
			}
		} else {
			noProgress = 0
		}
	}

	// Do not accept a body with extra bytes even when Content-Length lies.
	// This EOF probe requires a caller-enforced transport deadline.
	var extra [1]byte
	noProgress = 0
	for {
		if err := bodyGateState(ctx, revoked); err != nil {
			return nil, err
		}
		n, readErr := resp.Body.Read(extra[:])
		if err := bodyGateState(ctx, revoked); err != nil {
			return nil, err
		}
		if n != 0 {
			return nil, ErrUnsafeBody
		}
		if errors.Is(readErr, io.EOF) {
			if err := bodyGateState(ctx, revoked); err != nil {
				return nil, err
			}
			return buf, nil
		}
		if readErr != nil {
			return nil, ErrUnsafeBody
		}
		noProgress++
		if noProgress >= 100 {
			return nil, ErrUnsafeBody
		}
	}
}

func bodyGateState(ctx context.Context, revoked <-chan struct{}) error {
	select {
	case <-revoked:
		return ErrRevoked
	default:
	}
	return ctx.Err()
}
