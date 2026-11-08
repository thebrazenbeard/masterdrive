package mountgate

import (
    "errors"
    "strings"
    "testing"
)

func qualified() (Evidence, Pin) {
    digest := strings.Repeat("a", 64)
    e := Evidence{
        Origin: "https://source.example:443",
        TransportAuthenticated: true, DevicePaired: true,
        GrantCurrent: true, ReadOnly: true,
        ImmutableSnapshotVerified: true,
        StrongETag: "\"snapshot-17\"", SHA256: digest, Length: 4096,
    }
    return e, Pin{StrongETag:e.StrongETag, SHA256:e.SHA256, Length:e.Length}
}

func TestQualifiedSyntheticEvidence(t *testing.T) {
    e,p := qualified()
    if err := Check(e,p); err != nil { t.Fatalf("synthetic preflight: %v",err) }
}

func TestFailClosedOnMissingProofOrVersionDrift(t *testing.T) {
    cases := map[string]func(*Evidence,*Pin){
        "unpaired":func(e *Evidence,p *Pin){e.DevicePaired=false},
        "revoked":func(e *Evidence,p *Pin){e.GrantCurrent=false},
        "unauthenticated":func(e *Evidence,p *Pin){e.TransportAuthenticated=false},
        "writable":func(e *Evidence,p *Pin){e.ReadOnly=false},
        "mutable":func(e *Evidence,p *Pin){e.ImmutableSnapshotVerified=false},
        "etag drift":func(e *Evidence,p *Pin){e.StrongETag="\"snapshot-18\""},
        "weak etag":func(e *Evidence,p *Pin){e.StrongETag="W/\"snapshot-17\""},
        "digest drift":func(e *Evidence,p *Pin){e.SHA256=strings.Repeat("b",64)},
        "digest malformed":func(e *Evidence,p *Pin){e.SHA256=strings.Repeat("G",64)},
        "length drift":func(e *Evidence,p *Pin){e.Length++},
        "negative length":func(e *Evidence,p *Pin){e.Length=-1},
        "http":func(e *Evidence,p *Pin){e.Origin="http://source.example"},
        "userinfo":func(e *Evidence,p *Pin){e.Origin="https://user@source.example"},
        "query":func(e *Evidence,p *Pin){e.Origin="https://source.example/?x=1"},
        "fragment":func(e *Evidence,p *Pin){e.Origin="https://source.example/#x"},
        "path":func(e *Evidence,p *Pin){e.Origin="https://source.example/files"},
        "empty etag":func(e *Evidence,p *Pin){p.StrongETag=""},
        "empty digest":func(e *Evidence,p *Pin){p.SHA256=""},
    }
    for name, mutate := range cases {
        t.Run(name,func(t *testing.T){
            e,p:=qualified();mutate(&e,&p)
            if err:=Check(e,p); !errors.Is(err,ErrUnqualified) {
                t.Fatalf("expected fail closed, got %v",err)
            }
        })
    }
}
