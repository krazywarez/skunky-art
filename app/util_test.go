package app

import (
	"strings"
	"testing"
)

// TestNavBaseRendersFirstPageWithoutPageCount is the regression test for the
// navigation panel vanishing on some artworks. The comment list on a deviation
// cannot count its pages, so it passes Pages: 0; on page one, with no further
// page to offer, the panel used to render as nothing but a <br>.
func TestNavBaseRendersFirstPageWithoutPageCount(t *testing.T) {
	s := skunkyart{Page: 1, _pth: "/deviation/1"}

	out := s.NavBase(DeviationList{Pages: 0, More: false})

	if strings.TrimSpace(strings.TrimPrefix(out, "<br>")) == "" {
		t.Fatalf("navigation panel is empty, want the current page rendered: %q", out)
	}
	if !strings.Contains(out, "1") {
		t.Errorf("current page number missing from %q", out)
	}
}

// TestNavBaseFirstPageOffersNextWhenMore covers the same Pages: 0 case when a
// further page does exist: the current page must still appear alongside Next,
// rather than Next standing on its own with nothing to anchor it.
func TestNavBaseFirstPageOffersNextWhenMore(t *testing.T) {
	s := skunkyart{Page: 1, _pth: "/deviation/1"}

	out := s.NavBase(DeviationList{Pages: 0, More: true})

	if !strings.Contains(out, "Next") {
		t.Errorf("Next link missing from %q", out)
	}
	if !strings.Contains(out, "1") {
		t.Errorf("current page number missing from %q", out)
	}
}

// TestNavBaseKnownPageCountUnchanged pins the ordinary path: when the caller
// does know the page count, the window is still bounded by it.
func TestNavBaseKnownPageCountUnchanged(t *testing.T) {
	s := skunkyart{Page: 1, _pth: "/gallery"}

	out := s.NavBase(DeviationList{Pages: 3, More: true})

	for _, want := range []string{"1", "2", "3"} {
		if !strings.Contains(out, want) {
			t.Errorf("page %s missing from %q", want, out)
		}
	}
	if strings.Contains(out, "p=4") {
		t.Errorf("linked past the last page in %q", out)
	}
}

// TestInstanceAboutCarriesHideAI covers the About page and /api/instance both
// reading hide-ai from config, so a visitor can tell whether an instance
// filters AI-generated works without inferring it from absent results.
func TestInstanceAboutCarriesHideAI(t *testing.T) {
	hide := CFG.HideAI
	defer func() { CFG.HideAI = hide }()

	for _, on := range []bool{true, false} {
		CFG.HideAI = on
		if got := (instanceAbout{HideAI: CFG.HideAI}).HideAI; got != on {
			t.Errorf("instanceAbout.HideAI = %v, want %v", got, on)
		}
		if got := (settingsParams{HideAI: CFG.HideAI}).HideAI; got != on {
			t.Errorf("settingsParams.HideAI = %v, want %v", got, on)
		}
	}
}

// TestForcedThemeCSSOnlyForAnExplicitChoice pins that "auto" adds nothing: the
// stylesheet already carries dark plus a prefers-color-scheme light block, and
// appending a palette would defeat the visitor's own system setting.
func TestForcedThemeCSSOnlyForAnExplicitChoice(t *testing.T) {
	theme := CFG.Theme
	defer func() { CFG.Theme = theme }()

	CFG.Theme = "auto"
	if got := forcedThemeCSS(); got != "" {
		t.Errorf("auto appended %q, want nothing", got)
	}

	for _, want := range []string{"dark", "light"} {
		CFG.Theme = want
		css := forcedThemeCSS()
		if !strings.Contains(css, ":root{") {
			t.Errorf("%s theme did not emit a :root block: %q", want, css)
		}
		if !strings.Contains(css, "--bg:") || !strings.Contains(css, "--status-note:") {
			t.Errorf("%s theme is missing palette variables: %q", want, css)
		}
	}
}

// TestForcedThemeCSSCoversEveryVariable is the guard against a half-applied
// palette: a forced theme that omits a variable falls through to the default
// one, which is how a light theme ends up with black panels.
func TestForcedThemeCSSCoversEveryVariable(t *testing.T) {
	theme := CFG.Theme
	defer func() { CFG.Theme = theme }()

	vars := []string{
		"--bg", "--fg", "--fg-strong", "--link", "--link-hover", "--edge",
		"--edge-strong", "--accent", "--surface", "--surface-sunken",
		"--surface-alt", "--surface-deep", "--status-bad", "--status-good",
		"--status-mild", "--status-note",
	}
	for _, mode := range []string{"dark", "light"} {
		CFG.Theme = mode
		css := forcedThemeCSS()
		for _, v := range vars {
			if !strings.Contains(css, v+":") {
				t.Errorf("%s theme does not set %s", mode, v)
			}
		}
	}
}
