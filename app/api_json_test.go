package app

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/krazywarez/devianter"
)

// TestVisibleDeviationMatchesTheListingRules pins the single predicate the HTML
// listing and the JSON API both use. If these diverge, the API starts serving
// what the pages withhold.
func TestVisibleDeviationMatchesTheListingRules(t *testing.T) {
	nsfw, hide := CFG.Nsfw, CFG.HideAI
	defer func() { CFG.Nsfw, CFG.HideAI = nsfw, hide }()

	human := &devianter.Deviation{}
	robot := &devianter.Deviation{AI: true}
	adult := &devianter.Deviation{NSFW: true}

	CFG.Nsfw, CFG.HideAI = false, false
	if !VisibleDeviation(human) {
		t.Error("plain deviation hidden with both settings off")
	}
	if !VisibleDeviation(robot) {
		t.Error("AI deviation hidden while hide-ai is off")
	}
	if VisibleDeviation(adult) {
		t.Error("NSFW deviation shown while nsfw is off")
	}

	CFG.HideAI = true
	if VisibleDeviation(robot) {
		t.Error("AI deviation shown while hide-ai is on")
	}

	CFG.Nsfw = true
	if !VisibleDeviation(adult) {
		t.Error("NSFW deviation hidden while nsfw is on")
	}
}

// TestSearchRequiresAQuery covers the guard before any upstream request: an
// empty q must not become a search for the empty string.
func TestSearchRequiresAQuery(t *testing.T) {
	rec := httptest.NewRecorder()
	s := skunkyart{Writer: rec, Host: "http://localhost"}
	API{main: &s}.Search()

	if rec.Code != 400 {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "q") {
		t.Errorf("body does not name the missing parameter: %q", rec.Body.String())
	}
}

// TestSearchRejectsAnUnsupportedType stops an unknown letter reaching devianter.
func TestSearchRejectsAnUnsupportedType(t *testing.T) {
	rec := httptest.NewRecorder()
	s := skunkyart{Writer: rec, Host: "http://localhost", Query: "cats", Type: 'z'}
	API{main: &s}.Search()

	if rec.Code != 400 {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

// TestSearchGalleryTypeNeedsAUser: g and f are scoped to a user, and without one
// the upstream call is meaningless.
func TestSearchGalleryTypeNeedsAUser(t *testing.T) {
	for _, kind := range []rune{'g', 'f'} {
		rec := httptest.NewRecorder()
		s := skunkyart{Writer: rec, Host: "http://localhost", Query: "cats", Type: kind}
		s.Args = map[string][]string{}
		API{main: &s}.Search()

		if rec.Code != 400 {
			t.Errorf("type %c: status = %d, want 400", kind, rec.Code)
		}
	}
}

// TestPostRejectsANameWithoutAnID mirrors the HTML route, which pulls the
// deviation id out of the slug.
func TestPostRejectsANameWithoutAnID(t *testing.T) {
	rec := httptest.NewRecorder()
	s := skunkyart{Writer: rec, Host: "http://localhost"}
	API{main: &s}.Post("someone", "no-digits-here")

	if rec.Code != 400 {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

// TestToAPIDeviationShape is the contract consumers depend on: field names and
// the fact that media points back at this instance rather than at wixmp.
func TestToAPIDeviationShape(t *testing.T) {
	d := fullviewDeviation()
	d.ID = 42
	d.Title = "A Title"
	d.Author.Username = "alice"
	d.Stats.Favourites = 7
	d.Stats.Views = 99
	d.Extended.Tags = append(d.Extended.Tags, struct{ Name string }{Name: "cats"})

	s := skunkyart{Host: "http://localhost"}
	body, err := json.Marshal(s.toAPIDeviation(d))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for _, key := range []string{"id", "title", "author", "url", "nsfw", "ai", "daily_deviation", "favourites", "views"} {
		if _, ok := got[key]; !ok {
			t.Errorf("field %q missing from %s", key, body)
		}
	}
	if got["title"] != "A Title" || got["author"] != "alice" {
		t.Errorf("unexpected values in %s", body)
	}
	if tags, ok := got["tags"].([]any); !ok || len(tags) != 1 || tags[0] != "cats" {
		t.Errorf("tags not flattened to names: %s", body)
	}
}
