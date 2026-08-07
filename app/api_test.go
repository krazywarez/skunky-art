package app

import (
	"net/http/httptest"
	"testing"

	"github.com/krazywarez/devianter"
)

// fullviewDeviation returns a deviation whose media assembles into a non-empty
// wixmp URL, i.e. one that sendMedia is meant to serve.
func fullviewDeviation() *devianter.Deviation {
	d := &devianter.Deviation{}
	d.Media.BaseUri = "https://images-wixmp-abc.wixmp.com/f/u/x.png"
	d.Media.Name = "x"
	d.Media.Types = append(d.Media.Types, struct {
		T    string
		H, W int
	}{T: "fullview", H: 1920, W: 1280})
	return d
}

// TestSendMediaServesRealMedia is the regression test for the inverted guard: a
// deviation that has media must be sent, not dropped. In non-proxy mode that is
// a 302 to the wixmp URL; the pre-fix guard returned before writing anything.
func TestSendMediaServesRealMedia(t *testing.T) {
	proxy := CFG.Proxy
	CFG.Proxy = false
	defer func() { CFG.Proxy = proxy }()

	w := httptest.NewRecorder()
	API{main: &skunkyart{Writer: w}}.sendMedia(fullviewDeviation())

	if w.Code != 302 {
		t.Errorf("status is %d, want a 302 redirect to the media", w.Code)
	}
	if w.Header().Get("Location") == "" {
		t.Error("no Location header set — the media was dropped")
	}
}

// TestSendMediaIgnoresEmptyMedia pins the other half of the bug: a deviation
// with no media must be a no-op. With proxy on, the pre-fix code fell through to
// mediaURL[21:] on an empty string and panicked.
func TestSendMediaIgnoresEmptyMedia(t *testing.T) {
	proxy := CFG.Proxy
	CFG.Proxy = true
	defer func() { CFG.Proxy = proxy }()

	w := httptest.NewRecorder()
	API{main: &skunkyart{Writer: w}}.sendMedia(&devianter.Deviation{})

	if w.Code != 200 {
		t.Errorf("status is %d, want nothing written (recorder default 200)", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "" {
		t.Errorf("Location %q set for a media-less deviation, want none", loc)
	}
}
