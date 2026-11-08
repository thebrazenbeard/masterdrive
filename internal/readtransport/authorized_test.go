package readtransport

import (
 "context"
 "errors"
 "net/http"
 "testing"
)

type recordingTransport struct { calls int }
func (r *recordingTransport) RoundTrip(*http.Request) (*http.Response,error) {
 r.calls++
 return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)},nil
}

func TestAuthorizedTransportRejectsBypass(t *testing.T) {
 const authority = "node.tailnet.ts.net:443"
 client,err:=NewAuthorizedClient(authority)
 if err!=nil { t.Fatal(err) }
 guard,ok:=client.Transport.(authorizedRoundTripper)
 if !ok { t.Fatal("missing transport guard") }
 recorder:=&recordingTransport{}
 guard.base=recorder
 client.Transport=guard
 cases:=[]struct{name,rawURL,method string; mutate func(*http.Request)}{
  {"other host","https://evil.example/file","GET",nil},
  {"insecure","http://"+authority+"/file","GET",nil},
  {"post","https://"+authority+"/file","POST",nil},
  {"query","https://"+authority+"/file?secret=1","GET",nil},
  {"host override","https://"+authority+"/file","GET",func(r *http.Request){r.Host="evil.example"}},
  {"no path","https://"+authority,"GET",nil},
 }
 for _,tc:=range cases { t.Run(tc.name,func(t *testing.T){
  req,err:=http.NewRequestWithContext(context.Background(),tc.method,tc.rawURL,nil)
  if err!=nil {t.Fatal(err)}
  if tc.mutate!=nil {tc.mutate(req)}
  _,err=client.Do(req)
  if !errors.Is(err,ErrEndpoint){t.Fatalf("expected endpoint rejection: %v",err)}
 }) }
 if recorder.calls!=0 {t.Fatalf("unauthorized network calls: %d",recorder.calls)}
 req,err:=NewRequest(context.Background(),"https://"+authority+"/file",authority)
 if err!=nil {t.Fatal(err)}
 resp,err:=client.Do(req)
 if err!=nil {t.Fatal(err)}
 resp.Body.Close()
 if recorder.calls!=1 {t.Fatalf("authorized calls = %d",recorder.calls)}
}

func TestAuthorizedClientInvalidAuthority(t *testing.T) {
 for _,s:=range []string{"","evil.example/path","user@host","host?query","host#fragment","host."}{
  if _,err:=NewAuthorizedClient(s); !errors.Is(err,ErrEndpoint){t.Fatalf("%q: %v",s,err)}
 }
}
