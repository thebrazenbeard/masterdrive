package versionproof

import (
 "crypto/sha256"
 "errors"
 "math"
 "testing"
)

func TestVerifyWhole(t *testing.T) {
 data := []byte("source snapshot")
 pin := Pin{Size:uint64(len(data)), Digest:sha256.Sum256(data), Version:"snapshot:7"}
 if err:=VerifyWhole(pin,"snapshot:7",data); err!=nil {t.Fatal(err)}
 cases:=[]struct{name string; pin Pin; version string; data []byte}{
  {"missing pin version",Pin{Size:pin.Size,Digest:pin.Digest},"snapshot:7",data},
  {"missing observed version",pin,"",data},
  {"wrong version",pin,"snapshot:6",data},
  {"short content",pin,"snapshot:7",data[:len(data)-1]},
  {"changed content",pin,"snapshot:7",[]byte("source snapshOt")},
  {"changed digest",Pin{Size:pin.Size,Version:pin.Version},"snapshot:7",data},
 }
 for _,tc:=range cases {t.Run(tc.name,func(t *testing.T){if err:=VerifyWhole(tc.pin,tc.version,tc.data); !errors.Is(err,ErrInvalid){t.Fatalf("got %v",err)}})}
}
func TestCheckRangeBounds(t *testing.T) {
 p:=Pin{Size:100,Version:"v"}
 for _,r:=range [][2]uint64{{0,1},{99,1},{10,90}} {if err:=CheckRangeBounds(p,r[0],r[1]);err!=nil{t.Fatal(r,err)}}
 for _,r:=range [][2]uint64{{0,0},{100,1},{99,2},{math.MaxUint64,2},{1,math.MaxUint64}} {if err:=CheckRangeBounds(p,r[0],r[1]);!errors.Is(err,ErrInvalid){t.Fatalf("%v: %v",r,err)}}
 p.Version="";if err:=CheckRangeBounds(p,0,1);!errors.Is(err,ErrInvalid){t.Fatal(err)}
}
