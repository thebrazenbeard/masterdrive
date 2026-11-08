// Package revocation provides an experimental source-local, fail-closed
// revocation tombstone store. It is NOT wired into agent authorization and
// does not implement pairing, active-stream cancellation, or mount safety.
package revocation

import (
    "crypto/sha256"
    "crypto/rand"
    "encoding/hex"
    "errors"
    "fmt"
    "os"
    "path/filepath"
)

// Store owns a directory of one-way token-digest tombstones. Never put this
// directory on a shared or untrusted filesystem. Back up its contents before
// migration; losing the directory loses the revocation history.
type Store struct { dir string; identity os.FileInfo; marker string }

// Open requires an existing, owner-provisioned directory. It deliberately
// does not silently create an empty store when a directory is missing.
func Open(dir string) (*Store, error) {
    if !filepath.IsAbs(dir) { return nil, errors.New("revocation: absolute directory required") }
    info, err := os.Lstat(dir)
    if err != nil { return nil, fmt.Errorf("revocation: cannot inspect store: %w", err) }
    if !info.IsDir() { return nil, errors.New("revocation: store must be a real directory") }
    // The marker detects same-path directory replacement even where filesystem
    // identity values are reused (observed on Windows CI). This is NOT a
    // rollback-resistant or authenticated checkpoint.
    markerPath := filepath.Join(dir, ".store-identity")
    var marker string
    file, createErr := os.OpenFile(markerPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
    if createErr == nil {
        nonce := make([]byte, 32)
        if _, err := rand.Read(nonce); err != nil { file.Close(); return nil, err }
        marker = hex.EncodeToString(nonce)
        _, writeErr := file.WriteString(marker + "\\n")
        if writeErr == nil { writeErr = file.Sync() }
        closeErr := file.Close()
        if writeErr != nil { return nil, writeErr }
        if closeErr != nil { return nil, closeErr }
    } else if errors.Is(createErr, os.ErrExist) {
        b, err := os.ReadFile(markerPath)
        if err != nil { return nil, err }
        marker = string(b)
        if len(marker) != 65 || marker[64] != '\\n' { return nil, errors.New("revocation: invalid store identity") }
        if _, err := hex.DecodeString(marker[:64]); err != nil { return nil, errors.New("revocation: corrupt store identity") }
        marker = marker[:64]
    } else { return nil, createErr }
    return &Store{dir:dir, identity:info, marker:marker}, nil
}

func (s *Store) filename(token string) (string,error) {
    if len(token)<32 || len(token)>4096 { return "",errors.New("revocation: invalid token length") }
    digest:=sha256.Sum256([]byte(token))
    return filepath.Join(s.dir,hex.EncodeToString(digest[:])+".revoked"),nil
}

// Revoke creates a tombstone exclusively. An existing entry is not assumed
// valid: a concurrent writer may still be filling it, or it may be corrupt.
// This is NOT a transactional, rollback-resistant revocation ledger.
func (s *Store) Revoke(token string) error {
    name,err:=s.filename(token); if err!=nil{return err}
    if err:=s.ensureDirectory();err!=nil{return err}
    f,err:=os.OpenFile(name,os.O_WRONLY|os.O_CREATE|os.O_EXCL,0600)
    if errors.Is(err,os.ErrExist) {
        return s.verifyTombstone(name)
    }
    if err!=nil {return fmt.Errorf("revocation: create tombstone: %w",err)}
    _,writeErr:=f.Write([]byte("revoked\n"))
    if writeErr==nil {writeErr=f.Sync()}
    closeErr:=f.Close()
    if writeErr!=nil{return fmt.Errorf("revocation: persist tombstone: %w",writeErr)}
    if closeErr!=nil{return fmt.Errorf("revocation: close tombstone: %w",closeErr)}
    return nil
}

func (s *Store) ensureDirectory() error {
    info,err:=os.Lstat(s.dir)
    if err!=nil{return fmt.Errorf("revocation: inspect directory: %w",err)}
    if !info.IsDir() || !os.SameFile(s.identity, info) {return errors.New("revocation: directory unavailable or replaced")}
    b, err := os.ReadFile(filepath.Join(s.dir, ".store-identity"))
    if err != nil || string(b) != s.marker+"\\n" { return errors.New("revocation: store identity missing or replaced") }
    return nil
}

func (s *Store) verifyTombstone(name string) error {
    info,err:=os.Lstat(name)
    if err!=nil{return fmt.Errorf("revocation: inspect tombstone: %w",err)}
    if !info.Mode().IsRegular() || info.Size()!=int64(len("revoked\n")) {
        return errors.New("revocation: invalid tombstone")
    }
    // Lstat and ReadFile are not atomic against a hostile filesystem actor.
    // This package must never be used as an authorization oracle until
    // directory ownership, rollback protection and races are addressed.
    b,err:=os.ReadFile(name)
    if err!=nil{return fmt.Errorf("revocation: read tombstone: %w",err)}
    if string(b)!="revoked\n"{return errors.New("revocation: corrupt tombstone")}
    return nil
}

// Check returns revoked=true on any error. A missing tombstone is permitted
// only when the owner-provisioned directory remains accessible.
func (s *Store) Check(token string) (revoked bool,err error) {
    name,err:=s.filename(token);if err!=nil{return true,err}
    if err:=s.ensureDirectory();err!=nil{return true,err}
    _,err=os.Lstat(name)
    if errors.Is(err,os.ErrNotExist){return false,nil}
    if err!=nil{return true,err}
    if err:=s.verifyTombstone(name);err!=nil{return true,err}
    return true,nil
}
