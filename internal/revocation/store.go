// Package revocation provides an experimental source-local, fail-closed
// revocation tombstone store. It is NOT wired into agent authorization and
// does not implement pairing, active-stream cancellation, or mount safety.
package revocation

import (
 "crypto/sha256"
 "encoding/hex"
 "errors"
 "fmt"
 "os"
 "path/filepath"
)

// Store owns a directory of one-way token-digest tombstones. Never put this
// directory on a shared or untrusted filesystem. Back up its contents before
// migration; losing the directory loses the revocation history.
type Store struct { dir string }

// Open requires an existing, owner-provisioned directory. It deliberately
// does not silently create an empty store when a directory is missing.
func Open(dir string) (*Store, error) {
 if !filepath.IsAbs(dir) { return nil, errors.New("revocation: absolute directory required") }
 info, err := os.Stat(dir)
 if err != nil { return nil, fmt.Errorf("revocation: cannot inspect store: %w", err) }
 if !info.IsDir() { return nil, errors.New("revocation: store is not a directory") }
 return &Store{dir:dir}, nil
}

func (s *Store) filename(token string) (string,error) {
 if len(token)<32 || len(token)>4096 { return "",errors.New("revocation: invalid token length") }
 digest:=sha256.Sum256([]byte(token))
 return filepath.Join(s.dir,hex.EncodeToString(digest[:])+".revoked"),nil
}

// Revoke creates an immutable tombstone with O_EXCL; concurrent duplicate
// revocations are idempotent. Successful return means the file was written
// and Sync completed, not that the parent directory was durably synced.
func (s *Store) Revoke(token string) error {
 name,err:=s.filename(token); if err!=nil{return err}
 f,err:=os.OpenFile(name,os.O_WRONLY|os.O_CREATE|os.O_EXCL,0600)
 if errors.Is(err,os.ErrExist) { return nil }
 if err!=nil {return fmt.Errorf("revocation: create tombstone: %w",err)}
 _,writeErr:=f.Write([]byte("revoked\n"))
 if writeErr==nil {writeErr=f.Sync()}
 closeErr:=f.Close()
 if writeErr!=nil{return fmt.Errorf("revocation: persist tombstone: %w",writeErr)}
 if closeErr!=nil{return fmt.Errorf("revocation: close tombstone: %w",closeErr)}
 return nil
}

// Check returns revoked=true for any I/O error, including a missing or
// unreadable store. A genuinely absent tombstone is allowed only if the
// parent directory remains accessible. Callers MUST treat err as denial.
func (s *Store) Check(token string) (revoked bool,err error) {
 name,err:=s.filename(token);if err!=nil{return true,err}
 info,err:=os.Stat(s.dir);if err!=nil{return true,err}
 if !info.IsDir(){return true,errors.New("revocation: store unavailable")}
 _,err=os.Lstat(name)
 if err==nil{return true,nil}
 if errors.Is(err,os.ErrNotExist){return false,nil}
 return true,err
}
