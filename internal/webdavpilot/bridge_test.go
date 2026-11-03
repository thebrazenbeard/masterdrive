package webdavpilot

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const testHost = "0123456789abcdef0123456789abcdef"
const testRoot = "/dav/" + testHost + "/music"
const testFile = testRoot + "/song.txt"

type snapshot struct{ *bytes.Reader }
func (*snapshot) Close() error { return nil }

type testBackend struct {
	stat map[string]Metadata
	content map[string]string
	children []Metadata
	statErr error
	childrenErr error
	openErr error
	openCalls int
	childCalls int
	childLimitSeen int
	// Simulate a malformed adapter that ignores the caller's budget.
	childLimitIgnored bool
}

func (b *testBackend) Stat(_ context.Context, k Key) (Metadata, error) {
	if b.statErr != nil { return Metadata{}, b.statErr }
	v, ok := b.stat[k.Relative]
	if !ok { return Metadata{}, ErrNotFound }
	return v, nil
}
func (b *testBackend) Children(_ context.Context, _ Key, limit int) ([]Metadata, bool, error) {
	b.childCalls++
	b.childLimitSeen = limit
	if b.childrenErr != nil { return nil, false, b.childrenErr }
	if b.childLimitIgnored { return b.children, true, nil }
	if limit < 0 { return nil, false, errors.New("negative child budget") }
	if len(b.children) > limit { return b.children[:limit], false, nil }
	return b.children, true, nil
}
func (b *testBackend) OpenSnapshot(_ context.Context, k Key, expected string) (ReadSeekCloser, error) {
	b.openCalls++
	if b.openErr != nil { return nil, b.openErr }
	if b.stat[k.Relative].StrongETag != expected { return nil, ErrStale }
	return &snapshot{bytes.NewReader([]byte(b.content[k.Relative]))}, nil
}

func fixture() *testBackend {
	mtime := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	return &testBackend{
		stat: map[string]Metadata{
			"": {Name:"music", Directory:true, ModifiedAt:mtime},
			"song.txt": {Name:"song.txt", Size:6, StrongETag:`"version-1"`, ModifiedAt:mtime},
		},
		content: map[string]string{"song.txt":"abcdef"},
		children: []Metadata{{Name:"song.txt", Size:6, StrongETag:`"version-1"`, ModifiedAt:mtime}},
	}
}

func handler(t *testing.T, b Backend, allowed bool) *Handler {
	t.Helper()
	h, err := New(b, func(*http.Request, Key) bool { return allowed })
	if err != nil { t.Fatal(err) }
	return h
}

func call(h http.Handler, method, url string, headers map[string]string, body io.Reader) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, url, body)
	for k, v := range headers { r.Header.Set(k, v) }
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func local(path string) string { return "http://127.0.0.1:8822" + path }

func TestRequiresBackendAndAuthorization(t *testing.T) {
	if _, err := New(nil, nil); err == nil { t.Fatal("nil backend allowed") }
	if _, err := New(fixture(), nil); err == nil { t.Fatal("nil identity policy allowed") }
}

func TestAuthAndLocalHostGate(t *testing.T) {
	b := fixture()
	w := call(handler(t,b,false), http.MethodGet, local(testFile), nil,nil)
	if w.Code != http.StatusUnauthorized || b.openCalls != 0 { t.Fatalf("unauthorized read: %d opens=%d",w.Code,b.openCalls) }
	w = call(handler(t,b,true),http.MethodGet,"http://example.org"+testFile,nil,nil)
	if w.Code != http.StatusForbidden || b.openCalls != 0 { t.Fatalf("remote Host allowed: %d",w.Code) }
	for _, path := range []string{
		"/dav/"+testHost+"/music/../evil",
		"/dav/"+testHost+"/music/%2e%2e/evil",
		"/dav/"+testHost+"/music/%2fsong.txt",
		"/dav/"+testHost+"/music/%255csecret",
		"/dav/"+testHost+"/music/%5csecret",
		"/dav/"+testHost+"/music/song.txt:alternate-stream",
		"/dav/"+testHost+"/music/song.txt%3aalternate-stream",
		"/dav/"+testHost+"/music/CON.txt",
		"/dav/"+testHost+"/music/LPT1",
		"/dav/"+testHost+"/music/file.",
		"/dav/"+testHost+"/music/file%20",
		"/dav/"+testHost+"/music/caf%C3%A9",
		"/dav/"+testHost+"/CON/song.txt",
	} {
		w=call(handler(t,b,true),http.MethodGet,local(path),nil,nil)
		if w.Code != http.StatusNotFound { t.Errorf("unsafe path %q: %d",path,w.Code) }
	}
}

func TestReadOnlyAndOptions(t *testing.T) {
	b := fixture()
	h := handler(t,b,true)
	for _, method := range []string{http.MethodPut,http.MethodDelete,"MKCOL","MOVE","COPY","LOCK","UNLOCK","PROPPATCH",http.MethodPost} {
		w := call(h,method,local(testFile),nil,strings.NewReader("payload"))
		if w.Code != http.StatusMethodNotAllowed { t.Errorf("%s returned %d",method,w.Code) }
	}
	if b.openCalls != 0 || b.childCalls != 0 { t.Fatal("mutation touched backend") }
	w := call(h,http.MethodOptions,local(testRoot),nil,nil)
	if w.Code != http.StatusNoContent || w.Header().Get("DAV") != "1" || strings.Contains(w.Header().Get("Allow"),"PUT") {
		t.Fatalf("invalid options %d headers=%v",w.Code,w.Header())
	}
}

func TestReadRangeAndStableETag(t *testing.T) {
	b:=fixture()
	h:=handler(t,b,true)
	w:=call(h,http.MethodGet,local(testFile),nil,nil)
	if w.Code != http.StatusOK || w.Body.String() != "abcdef" || w.Header().Get("ETag") != `"version-1"` {
		t.Fatalf("whole read: %d %q etag=%q",w.Code,w.Body.String(),w.Header().Get("ETag"))
	}
	w=call(h,http.MethodHead,local(testFile),nil,nil)
	if w.Code != http.StatusOK || w.Body.Len()!=0 || w.Header().Get("Content-Length")!="6" { t.Fatalf("HEAD: %d %q",w.Code,w.Body.String()) }
	w=call(h,http.MethodHead,local(testFile),map[string]string{"Range":"bytes=0-1,3-4"},nil)
	if w.Code != http.StatusOK || w.Body.Len()!=0 || w.Header().Get("Content-Length")!="6" { t.Fatalf("HEAD must ignore Range: %d %q headers=%v",w.Code,w.Body.String(),w.Header()) }
	w=call(h,http.MethodHead,local(testFile),map[string]string{"Range":"bytes=0-1"},nil)
	if w.Code != http.StatusOK || w.Body.Len()!=0 || w.Header().Get("Content-Length")!="6" { t.Fatalf("HEAD must ignore valid Range too: %d %q headers=%v",w.Code,w.Body.String(),w.Header()) }
	w=call(h,http.MethodGet,local(testFile),map[string]string{"Range":"bytes=2-4","If-Range":`"version-1"`},nil)
	if w.Code != http.StatusPartialContent || w.Body.String() != "cde" || w.Header().Get("Content-Range") != "bytes 2-4/6" { t.Fatalf("range %d %q %v",w.Code,w.Body.String(),w.Header()) }
	w=call(h,http.MethodGet,local(testFile),map[string]string{"Range":"bytes=1-2,4-5"},nil)
	if w.Code == http.StatusOK || w.Code == http.StatusPartialContent { t.Errorf("multipart range accepted: %d",w.Code) }
	for _, validator := range []string{`"different"`, "Sun, 01 Nov 2026 10:00:00 GMT"} {
		w=call(h,http.MethodGet,local(testFile),map[string]string{"Range":"bytes=0-1","If-Range":validator},nil)
		if w.Code != http.StatusOK || w.Body.String() != "abcdef" || w.Header().Get("ETag") != `"version-1"` {
			t.Errorf("failed If-Range should return full representation, validator=%q status=%d body=%q",validator,w.Code,w.Body.String())
		}
	}
	w=call(h,http.MethodGet,local(testFile),map[string]string{"Range":"bytes=0-1","If-Range":"Mon, 02 Nov 2026 10:00:00 GMT"},nil)
	if w.Code != http.StatusPartialContent || w.Body.String() != "ab" {
		t.Errorf("matching date If-Range must preserve the requested range: status=%d body=%q",w.Code,w.Body.String())
	}
	w=call(h,http.MethodGet,local(testFile),map[string]string{"Range":"bytes=0-1,3-4","If-Range":`"different"`},nil)
	if w.Code != http.StatusOK || w.Body.String() != "abcdef" {
		t.Errorf("failed If-Range must ignore otherwise unsupported multipart range: status=%d body=%q",w.Code,w.Body.String())
	}
	w=call(h,http.MethodGet,local(testFile),map[string]string{"Range":"bytes=0-1,3-4","If-Range":`"version-1",suffix`},nil)
	if w.Code != http.StatusOK || w.Body.String() != "abcdef" {
		t.Errorf("malformed trailing If-Range suffix must not bypass single-range policy: status=%d body=%q",w.Code,w.Body.String())
	}
	w=call(h,http.MethodGet,local(testFile),map[string]string{"Range":"bytes=0-1,3-4","If-Range":` "version-1" `},nil)
	if w.Code != http.StatusBadRequest {
		t.Errorf("matching If-Range with whitespace must still reject multipart range: status=%d",w.Code)
	}
}

func TestOfflineStaleAndNoUnversionedFallback(t *testing.T) {
	for _, tc := range []struct{name string; configure func(*testBackend); want int; opens int}{
		{"offline",func(b *testBackend){b.statErr=ErrOffline},http.StatusServiceUnavailable,0},
		{"not found",func(b *testBackend){b.statErr=ErrNotFound},http.StatusNotFound,0},
		{"unversioned",func(b *testBackend){x:=b.stat["song.txt"];x.StrongETag="";b.stat["song.txt"]=x},http.StatusServiceUnavailable,0},
		{"misbound stat name",func(b *testBackend){x:=b.stat["song.txt"];x.Name="another.txt";b.stat["song.txt"]=x},http.StatusServiceUnavailable,0},
		{"stale after stat",func(b *testBackend){b.openErr=ErrStale},http.StatusPreconditionFailed,1},
		{"source offline after stat",func(b *testBackend){b.openErr=ErrOffline},http.StatusServiceUnavailable,1},
		{"snapshot length changed",func(b *testBackend){b.content["song.txt"]="wrong"},http.StatusBadGateway,1},
	} {
		t.Run(tc.name,func(t *testing.T){
			b:=fixture();tc.configure(b)
			w:=call(handler(t,b,true),http.MethodGet,local(testFile),nil,nil)
			if w.Code!=tc.want || b.openCalls!=tc.opens {t.Fatalf("status=%d want=%d opens=%d",w.Code,tc.want,b.openCalls)}
		})
	}
}

func TestPropfindDepthAndMultistatus(t *testing.T) {
	b:=fixture()
	h:=handler(t,b,true)
	for _, tc:=range []struct{depth string; hrefs int; childCalls int}{{"0",1,0},{"1",2,1}} {
		w:=call(h,"PROPFIND",local(testRoot),map[string]string{"Depth":tc.depth},nil)
		if w.Code!=207 {t.Fatalf("Depth %s status=%d: %s",tc.depth,w.Code,w.Body.String())}
		var report struct{
			XMLName xml.Name
			Responses []struct{Href string `xml:"href"`} `xml:"response"`
		}
		if err:=xml.Unmarshal(w.Body.Bytes(),&report);err!=nil {t.Fatalf("invalid XML: %v: %s",err,w.Body.String())}
		if report.XMLName.Space!="DAV:" || report.XMLName.Local!="multistatus" || len(report.Responses)!=tc.hrefs {
			t.Fatalf("invalid DAV multistatus: %+v",report)
		}
		if report.Responses[0].Href!=testRoot+"/" {t.Fatalf("bad root href: %+v",report.Responses)}
		if tc.hrefs==2 && report.Responses[1].Href!=testFile {t.Fatalf("bad child href: %+v",report.Responses)}
	}
	if b.childCalls!=1 {t.Fatalf("children calls=%d",b.childCalls)}
	if b.childLimitSeen!=maxChildren {t.Fatalf("unbounded backend request limit=%d",b.childLimitSeen)}
	for _, depth:=range []string{"","infinity","2"} {
		w:=call(h,"PROPFIND",local(testRoot),map[string]string{"Depth":depth},nil)
		if w.Code!=http.StatusForbidden {t.Errorf("Depth=%q allowed: %d",depth,w.Code)}
	}
	large:=fixture()
	for i:=0;i<=maxChildren;i++ {large.children=append(large.children,Metadata{Name:fmt.Sprintf("n%d",i),Directory:true})}
	w:=call(handler(t,large,true),"PROPFIND",local(testRoot),map[string]string{"Depth":"1"},nil)
	if w.Code!=http.StatusServiceUnavailable || strings.Contains(w.Body.String(),"multistatus") {t.Fatalf("unbounded listing returned %d",w.Code)}
	if large.childLimitSeen!=maxChildren {t.Fatalf("large listing requested limit=%d",large.childLimitSeen)}
	large.childLimitIgnored=true
	w=call(handler(t,large,true),"PROPFIND",local(testRoot),map[string]string{"Depth":"1"},nil)
	if w.Code!=http.StatusServiceUnavailable {t.Fatalf("backend ignoring child budget returned %d",w.Code)}
	exact:=fixture()
	for i:=1;i<maxChildren;i++ {exact.children=append(exact.children,Metadata{Name:fmt.Sprintf("ok%d",i),Directory:true})}
	w=call(handler(t,exact,true),"PROPFIND",local(testRoot),map[string]string{"Depth":"1"},nil)
	if w.Code!=207 {t.Fatalf("exactly bounded complete listing status=%d",w.Code)}
}

func TestPropfindChildValidationAndSourceFailure(t *testing.T) {
	for _, tc:=range []struct{name string; configure func(*testBackend);want int}{
		{"offline",func(b *testBackend){b.childrenErr=ErrOffline},http.StatusServiceUnavailable},
		{"invalid name",func(b *testBackend){b.children[0].Name="../escape"},http.StatusBadGateway},
		{"alternate stream",func(b *testBackend){b.children[0].Name="song.txt:payload"},http.StatusBadGateway},
		{"reserved device",func(b *testBackend){b.children[0].Name="COM1.txt"},http.StatusBadGateway},
		{"trailing dot",func(b *testBackend){b.children[0].Name="report."},http.StatusBadGateway},
		{"unicode policy pending",func(b *testBackend){b.children[0].Name="café.txt"},http.StatusBadGateway},
		{"file modtime missing",func(b *testBackend){b.children[0].ModifiedAt=time.Time{}},http.StatusBadGateway},
		{"unversioned child",func(b *testBackend){b.children[0].StrongETag=""},http.StatusBadGateway},
		{"case collision",func(b *testBackend){b.children=append(b.children,Metadata{Name:"SONG.TXT",Size:6,ModifiedAt:b.children[0].ModifiedAt,StrongETag:`"version-2"`})},http.StatusConflict},
	} {
		t.Run(tc.name,func(t *testing.T){
			b:=fixture();tc.configure(b)
			w:=call(handler(t,b,true),"PROPFIND",local(testRoot),map[string]string{"Depth":"1"},nil)
			if w.Code!=tc.want {t.Fatalf("status=%d want=%d",w.Code,tc.want)}
		})
	}
	b:=fixture()
	wrongRoot:=fixture()
	root := wrongRoot.stat[""]
	root.Name = "other-share"
	wrongRoot.stat[""] = root
	w:=call(handler(t,wrongRoot,true),"PROPFIND",local(testRoot),map[string]string{"Depth":"0"},nil)
	if w.Code!=http.StatusBadGateway {t.Fatalf("misbound root metadata allowed: %d",w.Code)}
	w=call(handler(t,b,true),"PROPFIND",local(testRoot),map[string]string{"Depth":"0"},strings.NewReader(`<D:propfind xmlns:D="DAV:"><D:allprop/></D:propfind>`))
	if w.Code!=207 {t.Fatalf("valid request XML rejected: %d: %s",w.Code,w.Body.String())}
	w=call(handler(t,b,true),"PROPFIND",local(testRoot),map[string]string{"Depth":"0"},strings.NewReader(`<evil/>`))
	if w.Code!=http.StatusBadRequest {t.Fatalf("wrong XML namespace accepted: %d",w.Code)}
}
