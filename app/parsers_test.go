package app

import (
	"strings"
	"testing"

	"github.com/krazywarez/devianter"
)

// aiListDevs returns one human-made and one AI-flagged deviation, each with a
// distinct title so the rendered output can be searched for either one.
func aiListDevs() []devianter.Deviation {
	human := devianter.Deviation{Title: "HumanArt"}
	human.Author.Username = "alice"

	robot := devianter.Deviation{Title: "RobotArt", AI: true}
	robot.Author.Username = "bob"

	return []devianter.Deviation{human, robot}
}

// TestDeviationListHidesAIWhenConfigured is the regression test for the hide-ai
// option: with it on, deviations flagged data.AI must not appear in a listing at
// all, while human-made ones are untouched.
func TestDeviationListHidesAIWhenConfigured(t *testing.T) {
	nsfw, hide := CFG.Nsfw, CFG.HideAI
	CFG.Nsfw, CFG.HideAI = true, true
	defer func() { CFG.Nsfw, CFG.HideAI = nsfw, hide }()

	out := skunkyart{Host: "http://localhost"}.DeviationList(aiListDevs(), false)

	if !strings.Contains(out, "HumanArt") {
		t.Error("human deviation was dropped, want it kept")
	}
	if strings.Contains(out, "RobotArt") {
		t.Error("AI deviation rendered while hide-ai is on, want it omitted")
	}
	if strings.Contains(out, "🤖") {
		t.Error("AI marker present while hide-ai is on")
	}
}

// TestDeviationListShowsAIByDefault pins down that the filter is opt-in: with
// hide-ai off the AI deviation is still listed and still carries its marker.
func TestDeviationListShowsAIByDefault(t *testing.T) {
	nsfw, hide := CFG.Nsfw, CFG.HideAI
	CFG.Nsfw, CFG.HideAI = true, false
	defer func() { CFG.Nsfw, CFG.HideAI = nsfw, hide }()

	out := skunkyart{Host: "http://localhost"}.DeviationList(aiListDevs(), false)

	if !strings.Contains(out, "RobotArt") {
		t.Error("AI deviation dropped while hide-ai is off, want it kept")
	}
	if !strings.Contains(out, "🤖") {
		t.Error("AI marker missing while hide-ai is off")
	}
}
