package grantproof

import (
 "bufio"
 "crypto/sha256"
 "crypto/subtle"
 "encoding/hex"
 "encoding/json"
 "errors"
 "io"
 "os"
 "path/filepath"
 "strings"
)

// JournalFingerprintFloors is a fail-closed filesystem prototype, NOT a
// production authorization store. It has no anti-rollback anchor, hardened
// filesystem ownership checks, or safe stale-lock recovery.
type JournalFingerprintFloors struct { Path string }
type journalEntry struct {
 Source string `json:"source"`
 Audience string `json:"audience"`
 Share string `json:"share"`
 Revision uint64 `json:"revision"`
 Fingerprint string `json:"fingerprint"`
 Checksum string `json:"checksum"`
}
func (e journalEntry) checksum() string {
 e.Checksum=""
 b,_:=json.Marshal(e)
 h:=sha256.Sum256(b)
 return hex.EncodeToString(h[:])
}
func (j *JournalFingerprintFloors) locked(fn func(map[relationshipKey]fingerprintRecord) error) error {
 if j==nil || j.Path=="" { return ErrFloorUnavailable }
 // Never remove an existing lock: it might belong to a live process.
 lock,err:=os.OpenFile(j.Path+".lock",os.O_CREATE|os.O_EXCL|os.O_WRONLY,0600)
 if err!=nil { return errors.Join(ErrFloorUnavailable,err) }
 defer func(){lock.Close();os.Remove(j.Path+".lock")}()
 f,err:=os.OpenFile(j.Path,os.O_CREATE|os.O_RDWR|os.O_APPEND,0600)
 if err!=nil {return errors.Join(ErrFloorUnavailable,err)}
 defer f.Close()
 records:=map[relationshipKey]fingerprintRecord{}
 // Bound both individual records and total replay work.
 const maxJournalBytes int64 = 8 << 20
 info,statErr:=f.Stat()
 if statErr!=nil || info.Size()>maxJournalBytes {return ErrFloorUnavailable}
 reader:=bufio.NewReaderSize(f,64*1024)
 for {
  lineBytes,readErr:=reader.ReadSlice('\n')
  line:=string(lineBytes)
  if readErr==io.EOF && line=="" {break}
  if readErr!=nil || !strings.HasSuffix(line,"\n") {return ErrFloorUnavailable}
  var e journalEntry
  if json.Unmarshal([]byte(line),&e)!=nil || e.Checksum!=e.checksum() {return ErrFloorUnavailable}
  key,ok:=relationship(e.Source,e.Audience,e.Share)
  raw,err:=hex.DecodeString(e.Fingerprint)
  if !ok || err!=nil || len(raw)!=32 || e.Revision==0 {return ErrFloorUnavailable}
  var fp [32]byte
  copy(fp[:],raw)
  old,exists:=records[key]
  if exists && (e.Revision<old.revision || (e.Revision==old.revision && subtle.ConstantTimeCompare(old.fingerprint[:],fp[:])!=1)) {return ErrFloorUnavailable}
  records[key]=fingerprintRecord{e.Revision,fp}
 }
 return fn(records)
}
func (j *JournalFingerprintFloors) append(e journalEntry) error {
 e.Checksum=e.checksum()
 b,err:=json.Marshal(e);if err!=nil{return ErrFloorUnavailable}
 f,err:=os.OpenFile(j.Path,os.O_APPEND|os.O_WRONLY,0600)
 if err!=nil{return errors.Join(ErrFloorUnavailable,err)}
 defer f.Close()
 b=append(b,'\n')
 // Enforce the aggregate limit on append as well as replay. The caller
 // holds the exclusive cooperative journal lock for this operation.
 const maxJournalBytes int64 = 8 << 20
 info,err:=f.Stat()
 if err!=nil || info.Size()>maxJournalBytes || int64(len(b))>maxJournalBytes-info.Size() {return ErrFloorUnavailable}
 n,err:=f.Write(b)
 if err!=nil || n!=len(b) {return ErrFloorUnavailable}
 if err=f.Sync();err!=nil{return errors.Join(ErrFloorUnavailable,err)}
 return nil
}
func (j *JournalFingerprintFloors) Provision(source,audience,share string,revision uint64,fp [32]byte) error {
 key,ok:=relationship(source,audience,share);if !ok || revision==0{return ErrFloorUnavailable}
 return j.locked(func(m map[relationshipKey]fingerprintRecord)error{
  if _,exists:=m[key];exists{return ErrFloorUnavailable}
  return j.append(journalEntry{Source:source,Audience:audience,Share:share,Revision:revision,Fingerprint:hex.EncodeToString(fp[:])})
 })
}
func (j *JournalFingerprintFloors) FingerprintFloor(source,audience,share string)(uint64,[32]byte,error){
 key,ok:=relationship(source,audience,share);if !ok{return 0,[32]byte{},ErrFloorUnavailable}
 var r fingerprintRecord
 err:=j.locked(func(m map[relationshipKey]fingerprintRecord)error{
  var exists bool;r,exists=m[key];if !exists{return ErrFloorUnavailable};return nil
 })
 return r.revision,r.fingerprint,err
}
func (j *JournalFingerprintFloors) CommitFingerprintIfCurrent(source,audience,share string,revision uint64,fp [32]byte)error{
 key,ok:=relationship(source,audience,share);if !ok || revision==0{return ErrFloorUnavailable}
 return j.locked(func(m map[relationshipKey]fingerprintRecord)error{
  old,exists:=m[key];if !exists || revision<old.revision{return ErrFloorUnavailable}
  if revision==old.revision {
   if subtle.ConstantTimeCompare(old.fingerprint[:],fp[:])!=1{return ErrCheckpointEquivocation}
   return nil
  }
  return j.append(journalEntry{Source:source,Audience:audience,Share:share,Revision:revision,Fingerprint:hex.EncodeToString(fp[:])})
 })
}
// Keep path selection with the caller; never automatically create parent dirs.
var _ = filepath.Separator
