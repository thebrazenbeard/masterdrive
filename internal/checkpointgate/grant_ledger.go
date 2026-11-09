package checkpointgate

import (
 "crypto/sha256"
 "encoding/binary"
 "sort"
)

// GrantRecord is a deliberately narrow prototype of a source-owned grant.
// It is NOT a verified Tailscale identity or a production authorization schema.
type GrantRecord struct {
 ID string
 PeerID string
 ShareID string
 Epoch uint64
}

// DigestGrantLedger hashes both active grants and revocation tombstones.
// This is an unwired schema prototype: it excludes expiry, key versions,
// signatures, share capabilities and any other fields a production ledger needs.
// Never use its result to authorize a read, pairing or mount.
func DigestGrantLedger(storeID string, revision uint64, grants []GrantRecord, tombstones []Revocation) ([32]byte,error) {
 var zero [32]byte
 if storeID == "" || len(storeID)>65535 || revision==0 || len(grants)>1000000 || len(tombstones)>1000000 { return zero,ErrInvalidLedger }
 g:=append([]GrantRecord(nil),grants...)
 t:=append([]Revocation(nil),tombstones...)
 sort.Slice(g,func(i,j int)bool{return g[i].ID<g[j].ID})
 sort.Slice(t,func(i,j int)bool{return t[i].ID<t[j].ID})
 for i,v:=range g {
  if v.ID=="" || v.PeerID=="" || v.ShareID=="" || v.Epoch==0 || len(v.ID)>65535 || len(v.PeerID)>65535 || len(v.ShareID)>65535 || (i>0 && g[i-1].ID==v.ID) {return zero,ErrInvalidLedger}
 }
 for i,v:=range t {
  if v.ID=="" || v.Epoch==0 || len(v.ID)>65535 || (i>0 && t[i-1].ID==v.ID) {return zero,ErrInvalidLedger}
 }
 // A revoked grant cannot simultaneously be active under the same ID.
 i,j:=0,0
 for i<len(g) && j<len(t) {
  if g[i].ID==t[j].ID {return zero,ErrInvalidLedger}
  if g[i].ID<t[j].ID {i++} else {j++}
 }
 h:=sha256.New()
 h.Write([]byte("masterdrive/grant-ledger-prototype/v1\x00"))
 var b [8]byte
 writeUint:=func(v uint64){binary.BigEndian.PutUint64(b[:],v);h.Write(b[:])}
 writeString:=func(s string){writeUint(uint64(len(s)));h.Write([]byte(s))}
 writeString(storeID);writeUint(revision)
 writeUint(uint64(len(g)))
 for _,v:=range g {writeString(v.ID);writeString(v.PeerID);writeString(v.ShareID);writeUint(v.Epoch)}
 writeUint(uint64(len(t)))
 for _,v:=range t {writeString(v.ID);writeUint(v.Epoch)}
 var out [32]byte
 copy(out[:],h.Sum(nil))
 return out,nil
}
