package app

import (
	"encoding/json"
	"regexp"

	"github.com/krazywarez/devianter"
)

// The API deliberately serves its own shapes rather than devianter's structs.
// Those describe DeviantArt's payloads and change when DeviantArt changes; an
// instance's consumers should not have to.
type apiDeviation struct {
	ID        int      `json:"id"`
	Title     string   `json:"title"`
	Author    string   `json:"author"`
	URL       string   `json:"url"`
	Published string   `json:"published,omitempty"`
	NSFW      bool     `json:"nsfw"`
	AI        bool     `json:"ai"`
	DailyDev  bool     `json:"daily_deviation"`
	Tags      []string `json:"tags,omitempty"`
	Preview   string   `json:"preview,omitempty"`
	Fullview  string   `json:"fullview,omitempty"`
	Favourite int      `json:"favourites"`
	Views     int      `json:"views"`
}

type apiSearchResponse struct {
	Query   string         `json:"query"`
	Type    string         `json:"type"`
	Page    int            `json:"page"`
	Results []apiDeviation `json:"results"`
}

// toAPIDeviation flattens one deviation. Media URLs are routed back through this
// instance when proxying is on, so a consumer never has to talk to wixmp itself
// — the same indirection the HTML pages use.
func (s skunkyart) toAPIDeviation(d *devianter.Deviation) apiDeviation {
	out := apiDeviation{
		ID:        d.ID,
		Title:     d.Title,
		Author:    d.Author.Username,
		URL:       ConvertDeviantArtURLToSkunkyArt(s.Host, d.Url),
		NSFW:      d.NSFW,
		AI:        d.AI,
		DailyDev:  d.DD,
		Preview:   ParseMedia(s.Host, d.Media, 320),
		Fullview:  ParseMedia(s.Host, d.Media),
		Favourite: d.Stats.Favourites,
		Views:     d.Stats.Views,
	}
	if !d.PublishedTime.IsZero() {
		out.Published = d.PublishedTime.UTC().Format("2006-01-02T15:04:05Z")
	}
	for _, t := range d.Extended.Tags {
		out.Tags = append(out.Tags, t.Name)
	}
	return out
}

// writeJSON marshals v. A marshal failure is reported as a 500 rather than
// sending a half-written body with a 200 already on the wire.
func (a API) writeJSON(v any) {
	body, err := json.Marshal(v)
	if err != nil {
		a.Error("failed to encode response", 500)
		return
	}
	_, _ = a.main.Writer.Write(body)
}

// Search responds with the deviations matching ?q=, honouring the instance's
// NSFW and hide-ai settings through VisibleDeviation — the same rule the HTML
// listing applies.
//
// ?type= takes the same single letters the pages do: a (art, default),
// t (text), g (gallery), f (favourites). Gallery and favourites need ?usr=.
func (a API) Search() {
	s := a.main
	if s.Query == "" {
		a.Error("missing required parameter: q", 400)
		return
	}

	kind := s.Type
	if kind == 0 {
		kind = 'a'
	}

	var (
		result  devianter.Search
		daError devianter.Error
		err     error
	)
	switch kind {
	case 'a', 't':
		result, daError, err = devianter.PerformSearch(s.Query, s.Page, kind)
	case 'g', 'f':
		usr := s.Args.Get("usr")
		if usr == "" {
			a.Error("type "+string(kind)+" requires the usr parameter", 400)
			return
		}
		result, daError, err = devianter.PerformSearch(s.Query, s.Page, kind, usr)
	default:
		a.Error("unsupported type: "+string(kind), 400)
		return
	}

	if err != nil {
		a.Error("upstream request failed", 502)
		return
	}
	if daError.RAW != nil {
		a.Error("deviantart returned an error", 502)
		return
	}

	// Non-nil so an empty page marshals as [] rather than null.
	out := apiSearchResponse{
		Query:   s.Query,
		Type:    string(kind),
		Page:    s.Page,
		Results: []apiDeviation{},
	}
	for i := range result.Results {
		d := &result.Results[i]
		if !VisibleDeviation(d) {
			continue
		}
		out.Results = append(out.Results, s.toAPIDeviation(d))
	}
	a.writeJSON(out)
}

// Post responds with a single deviation. postname carries the numeric id the
// way the HTML route does, e.g. "some-title-123456789".
//
// Gated on NSFW only, matching the page: hide-ai omits AI work from *listings*,
// and a reader who has followed a direct link to one still gets it. Diverging
// here would make the API disagree with the site it fronts.
func (a API) Post(author, postname string) {
	s := a.main
	if author == "" || postname == "" {
		a.Error("missing author or post name", 400)
		return
	}

	idSearch := regexp.MustCompile("[0-9]+").FindAllString(postname, -1)
	if len(idSearch) < 1 {
		a.Error("post name carries no deviation id", 400)
		return
	}

	post, daError := devianter.GetDeviation(idSearch[len(idSearch)-1], author)
	if daError.RAW != nil {
		a.Error("deviantart returned an error", 502)
		return
	}

	d := &post.Deviation
	if d.NSFW && !CFG.Nsfw {
		a.Error("nsfw content is disabled on this instance", 403)
		return
	}

	a.writeJSON(struct {
		apiDeviation
		Description string `json:"description,omitempty"`
		Downloads   int    `json:"downloads"`
		Filesize    int    `json:"filesize,omitempty"`
		Width       int    `json:"width,omitempty"`
		Height      int    `json:"height,omitempty"`
	}{
		apiDeviation: s.toAPIDeviation(d),
		Description:  ParseDescription(s.Host, d.Extended.DescriptionText),
		Downloads:    d.Stats.Downloads,
		Filesize:     d.Extended.OriginalFile.Filesize,
		Width:        d.Extended.OriginalFile.Width,
		Height:       d.Extended.OriginalFile.Height,
	})
}
