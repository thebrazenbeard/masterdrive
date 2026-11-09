package checkpointgate

import (
 "bytes"
 "errors"
 "testing"
)

func TestVerifyExact(t *testing.T) {
 good := Checkpoint{StoreID:"store-a", Revision:7, Digest:bytes.Repeat([]byte{0x42},32)}
 clone := func() Checkpoint { return Checkpoint{StoreID:good.StoreID,Revision:good.Revision,Digest:append([]byte(nil),good.Digest...)} }
 if err:=VerifyExact(good,clone()); err!=nil { t.Fatalf("matching independent checkpoint: %v",err) }
 cases:=map[string]func(*Checkpoint){
  "missing store":func(c *Checkpoint){c.StoreID=""},
  "different store":func(c *Checkpoint){c.StoreID="store-b"},
  "zero revision":func(c *Checkpoint){c.Revision=0},
  "rollback revision":func(c *Checkpoint){c.Revision--},
  "unexpected advance":func(c *Checkpoint){c.Revision++},
  "missing digest":func(c *Checkpoint){c.Digest=nil},
  "short digest":func(c *Checkpoint){c.Digest=c.Digest[:31]},
  "changed tombstone digest":func(c *Checkpoint){c.Digest[31]^=1},
 }
 for name,mutate:=range cases {t.Run(name,func(t *testing.T){
  observed:=clone();mutate(&observed)
  if err:=VerifyExact(good,observed);!errors.Is(err,ErrUntrustedCheckpoint){t.Fatalf("got %v, want checkpoint rejection",err)}
 })}
 t.Run("missing trusted evidence",func(t *testing.T){
  if err:=VerifyExact(Checkpoint{},good);!errors.Is(err,ErrUntrustedCheckpoint){t.Fatalf("got %v",err)}
 })
 t.Run("trusted digest not canonical length",func(t *testing.T){
  trusted:=clone();trusted.Digest=append(trusted.Digest,0)
  if err:=VerifyExact(trusted,good);!errors.Is(err,ErrUntrustedCheckpoint){t.Fatalf("got %v",err)}
 })
}
