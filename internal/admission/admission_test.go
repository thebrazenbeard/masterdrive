package admission

import ("errors"; "testing"; "time")

func valid() (Evidence,time.Time) {
 now:=time.Date(2026,11,9,16,0,0,0,time.UTC)
 e:=Evidence{PeerID:"peer-A",PairedPeerID:"peer-A",GrantPeerID:"peer-A",ShareID:"share-1",GrantShareID:"share-1",PeerKeyVersion:2,PairedKeyVersion:2,GrantReadOnly:true,GrantExpires:now.Add(time.Minute),GrantEpoch:7,CurrentPolicyEpoch:7,SnapshotVersion:"v4",PinnedVersion:"v4",SnapshotSize:3,PinnedSize:3,SourceRootConfined:true,ImmutableSnapshot:true,TransportAuthenticated:true,TransportDeadlineEnforced:true}
 e.SnapshotDigest[0]=1;e.PinnedDigest=e.SnapshotDigest
 return e,now
}
func TestFailClosedAdmission(t *testing.T) {
 e,now:=valid();if err:=Check(e,now);err!=nil {t.Fatalf("valid: %v",err)}
 cases:=map[string]func(*Evidence){
 "unpaired":func(e *Evidence){e.PairedPeerID=""}, "different peer":func(e *Evidence){e.GrantPeerID="peer-B"},
 "wrong share":func(e *Evidence){e.GrantShareID="share-2"}, "rotated key":func(e *Evidence){e.PairedKeyVersion++},
 "revoked":func(e *Evidence){e.Revoked=true}, "write grant":func(e *Evidence){e.GrantReadOnly=false},
 "stale epoch":func(e *Evidence){e.CurrentPolicyEpoch++}, "expired":func(e *Evidence){e.GrantExpires=now},
 "wrong version":func(e *Evidence){e.PinnedVersion="v5"}, "wrong size":func(e *Evidence){e.PinnedSize++},
 "wrong digest":func(e *Evidence){e.PinnedDigest[0]++},
 "zero digest":func(e *Evidence){e.PinnedDigest=[32]byte{};e.SnapshotDigest=[32]byte{}},
 "root escape":func(e *Evidence){e.SourceRootConfined=false}, "mutable":func(e *Evidence){e.ImmutableSnapshot=false},
 "unauthenticated":func(e *Evidence){e.TransportAuthenticated=false},
 "unbounded":func(e *Evidence){e.TransportDeadlineEnforced=false},
 }
 for name,mutate:=range cases {t.Run(name,func(t *testing.T){bad:=e;mutate(&bad);if !errors.Is(Check(bad,now),ErrDenied){t.Fatal("admitted invalid evidence")}})}
 if !errors.Is(Check(e,time.Time{}),ErrDenied){t.Fatal("zero clock admitted")}
}
