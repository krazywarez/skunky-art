package app

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// resetMemCache empties the in-memory cache so each test starts clean.
func resetMemCache() {
	mx.Lock()
	defer mx.Unlock()
	tempFS = make(map[[20]byte]*file)
}

func key(b byte) [20]byte {
	var k [20]byte
	k[0] = b
	return k
}

// TestBuildMediaURLRejectsForgedSubdomain is the regression test for the SSRF in
// the media proxy: subdomain reaches us percent-decoded from the request path,
// so it can carry "@", "#", "?" and "/" — every character that ends a host. When
// the URL was built by concatenation, each of these reparsed as a host the
// caller chose. The label is the host, so it has to be rejected, not escaped.
func TestBuildMediaURLRejectsForgedSubdomain(t *testing.T) {
	// The path a request for /media/file/<subdomain>/f.jpg would decode to.
	for _, subdomain := range []string{
		"x@attacker.example#",     // userinfo + fragment: host is attacker.example
		"x@attacker.example/",     // userinfo, host terminated by the slash
		"x@127.0.0.1:8080/",       // the same, aimed inside the instance's network
		"x@[::1]:8080/",           // IPv6 loopback
		"attacker.example#",       // fragment alone truncates to images-wixmp-attacker.example
		"attacker.example?",       // query does the same
		"a/../../secret",          // slashes escape the label entirely
		"a\\attacker.example",     // backslash, which some parsers fold to "/"
		"a.wixmp.com.attacker.eu", // dots: a label may not contain them
		"",                        // empty label
	} {
		if got, ok := buildMediaURL(subdomain, "f/x.jpg", ""); ok {
			t.Errorf("subdomain %q: accepted and built %q, want rejected", subdomain, got)
		}
	}
}

// makeMediaToken builds a JWT-shaped token whose obj carries the given blur
// value, mirroring the wixmp media tokens DeviantArt signs. Pass a ">=N" string
// for a blur-constrained (mature) token, or nil for an unconstrained one.
func makeMediaToken(t *testing.T, blur any) string {
	t.Helper()
	claims := map[string]any{
		"obj": [][]map[string]any{{{"path": "/f/x.png", "blur": blur}}},
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	enc := base64.RawURLEncoding.EncodeToString
	return enc([]byte(`{"alg":"none"}`)) + "." + enc(payload) + ".sig"
}

// TestBlurConstraint is the regression test for the mature-media 403: a token
// whose obj demands a blur must yield that radius, and anything else must yield
// 0 so the transform is left untouched.
func TestBlurConstraint(t *testing.T) {
	if got := blurConstraint(makeMediaToken(t, ">=10")); got != 10 {
		t.Errorf("blur-constrained token: got %d, want 10", got)
	}
	if got := blurConstraint(makeMediaToken(t, nil)); got != 0 {
		t.Errorf("null-blur token: got %d, want 0", got)
	}
	// Nothing parseable as a claims payload: fail open, leaving the URL alone.
	for _, tok := range []string{"", "not-a-jwt", "a.b", "a.!!!.c"} {
		if got := blurConstraint(tok); got != 0 {
			t.Errorf("unparseable token %q: got %d, want 0", tok, got)
		}
	}
}

// TestAddBlurToTransform checks the string surgery: a blur op is inserted into a
// /v1/fit transform, paths without one are untouched, and an existing op is not
// doubled.
func TestAddBlurToTransform(t *testing.T) {
	got := addBlurToTransform("f/u/x.png/v1/fit/w_1280,h_1920/x.png", 10)
	if want := "f/u/x.png/v1/fit/w_1280,h_1920,blur_10/x.png"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := addBlurToTransform("f/u/x.gif", 10); got != "f/u/x.gif" {
		t.Errorf("path without a transform was modified: %q", got)
	}
	blurred := "f/u/x.png/v1/fit/w_1280,h_1920,blur_10/x.png"
	if got := addBlurToTransform(blurred, 10); got != blurred {
		t.Errorf("existing blur op was doubled: %q", got)
	}
}

// TestBuildMediaURLAddsBlurWhenTokenDemandsIt drives the whole path: a
// blur-constrained token gains a matching blur op in the composed URL, and an
// unconstrained one does not.
func TestBuildMediaURLAddsBlurWhenTokenDemandsIt(t *testing.T) {
	path := "f/u/x.png/v1/fit/w_1280,h_1920/x.png"

	got, ok := buildMediaURL("ed30a86b", path, makeMediaToken(t, ">=10"))
	if !ok {
		t.Fatal("a plain label was rejected, want accepted")
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("built an unparseable URL %q: %v", got, err)
	}
	if !strings.Contains(u.Path, "w_1280,h_1920,blur_10") {
		t.Errorf("transform is %q, want a blur_10 op added", u.Path)
	}

	if got, _ := buildMediaURL("ed30a86b", path, makeMediaToken(t, nil)); strings.Contains(got, "blur") {
		t.Errorf("unconstrained media gained a blur op: %q", got)
	}
}

// TestBuildMediaURLKeepsHostOnWixmp is the property that actually matters: for
// anything accepted, the host the client ends up talking to is the CDN.
func TestBuildMediaURLKeepsHostOnWixmp(t *testing.T) {
	got, ok := buildMediaURL("ed30a86b-8c4c-a887", "f/x.jpg", "abc")
	if !ok {
		t.Fatal("a plain hex-and-dash label was rejected, want accepted")
	}

	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("built an unparseable URL %q: %v", got, err)
	}
	if u.Host != "images-wixmp-ed30a86b-8c4c-a887.wixmp.com" {
		t.Errorf("host is %q, want the wixmp CDN", u.Host)
	}
	if u.User != nil {
		t.Errorf("URL carries userinfo %v, want none", u.User)
	}
	if u.Query().Get("token") != "abc" {
		t.Errorf("token is %q, want abc", u.Query().Get("token"))
	}
}

// TestBuildMediaURLEscapesPath checks that the path cannot end the URL early and
// smuggle in a query or fragment of the caller's choosing.
func TestBuildMediaURLEscapesPath(t *testing.T) {
	got, ok := buildMediaURL("ed30a86b", "f/x.jpg#frag?q=1", "")
	if !ok {
		t.Fatal("a plain label was rejected, want accepted")
	}

	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("built an unparseable URL %q: %v", got, err)
	}
	if u.Fragment != "" {
		t.Errorf("path opened a fragment %q, want it escaped into the path", u.Fragment)
	}
	if u.RawQuery != "" {
		t.Errorf("path opened a query %q, want it escaped into the path", u.RawQuery)
	}
	if u.Path != "/f/x.jpg#frag?q=1" {
		t.Errorf("path is %q, want it preserved verbatim", u.Path)
	}
}

// TestDownloadAndSendMediaRejectsForgedSubdomain drives the handler itself, to
// pin down that a forged label is refused before any fetch is attempted rather
// than merely being rejected by the helper. Proxying is enabled here, so the
// pre-fix handler would have reached the network on this input.
func TestDownloadAndSendMediaRejectsForgedSubdomain(t *testing.T) {
	proxy := CFG.Proxy
	CFG.Proxy = true
	defer func() { CFG.Proxy = proxy }()

	w := httptest.NewRecorder()
	s := skunkyart{Writer: w, Host: "http://localhost", Args: url.Values{}}
	s.DownloadAndSendMedia("x@127.0.0.1:8080/", "f/x.jpg")

	if w.Code != 400 {
		t.Errorf("status is %d, want 400 for a forged subdomain", w.Code)
	}
}

// TestMemCacheConcurrentAccess hammers the in-memory cache from many goroutines
// while the janitor ages it, which is what a media flood does on an instance
// with memcache enabled.
//
// This is the regression test for the readers that touched tempFS without
// holding mx: concurrently with the janitor's delete that is a concurrent map
// read and map write, which the runtime reports as a fatal error that no
// recover can catch. Run under -race to also catch the unsynchronised field
// access that does not happen to trip the map check.
func TestMemCacheConcurrentAccess(t *testing.T) {
	resetMemCache()
	defer resetMemCache()

	const workers, rounds = 24, 200
	body := []byte("not-really-an-image")

	var wg sync.WaitGroup
	for w := range workers {
		wg.Go(func() {
			for i := range rounds {
				// Overlapping keys, so goroutines contend for the same entries.
				k := key(byte((w + i) % 8)) //nolint:gosec // G115: (w+i)%8 is 0-7
				memPut(k, body)
				memGet(k)
			}
		})
	}

	// Age the cache underneath the readers and writers: this is the delete that
	// the old per-entry goroutines raced against.
	wg.Go(func() {
		for range rounds {
			ageMemCache()
		}
	})

	wg.Wait()
}

// TestMemGetReturnsStoredBody covers the plain hit and miss paths.
func TestMemGetReturnsStoredBody(t *testing.T) {
	resetMemCache()
	defer resetMemCache()

	k := key(1)
	if got := memGet(k); got != nil {
		t.Fatalf("empty cache: got %q, want nil", got)
	}

	want := []byte("body")
	memPut(k, want)

	got := memGet(k)
	if !bytes.Equal(got, want) {
		t.Fatalf("after put: got %q, want %q", got, want)
	}
}

// TestMemPutIgnoresEmptyBody stops a failed fetch from caching a zero-length
// image that would then be served to everyone until it aged out.
func TestMemPutIgnoresEmptyBody(t *testing.T) {
	resetMemCache()
	defer resetMemCache()

	k := key(2)
	memPut(k, nil)
	memPut(k, []byte{})

	if got := memGet(k); got != nil {
		t.Fatalf("empty body was cached: got %q, want nil", got)
	}
}

// TestAgeMemCacheEvicts checks that a cold entry is dropped while a hot one
// survives, since that scoring is the only bound on the cache's memory use.
func TestAgeMemCacheEvicts(t *testing.T) {
	resetMemCache()
	defer resetMemCache()

	cold, hot := key(3), key(4)
	memPut(cold, []byte("cold"))
	memPut(hot, []byte("hot"))

	// A hit raises the hot entry's score above zero.
	memGet(hot)

	ageMemCache()

	if got := memGet(cold); got != nil {
		t.Errorf("cold entry survived aging: got %q, want nil", got)
	}
	if memGet(hot) == nil {
		t.Error("hot entry was evicted after a hit, want it kept")
	}
}
