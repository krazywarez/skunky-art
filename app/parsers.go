package app

import (
	"encoding/json"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/krazywarez/devianter"
	"golang.org/x/net/html"
)

// ParseComments renders a comment thread, nesting replies under the comment they
// answer. It returns a placeholder message rather than failing if the upstream
// fetch errored.
func (s skunkyart) ParseComments(c devianter.Comments, daError devianter.Error) string {
	if daError.RAW != nil {
		return esc(T(s.Lang, "comments.failed"))
	}

	var cmmts strings.Builder
	replied := make(map[int]string)

	cmmts.WriteString("<details><summary>")
	cmmts.WriteString(esc(T(s.Lang, "deviation.comments")))
	cmmts.WriteString(": <b>")
	cmmts.WriteString(strconv.Itoa(c.Total))
	cmmts.WriteString("</b></summary>")
	for _, x := range c.Thread {
		replied[x.ID] = x.User.Username
		cmmts.WriteString(`<div class="msg`)
		if x.Parent > 0 {
			cmmts.WriteString(` reply`)
		}
		cmmts.WriteString(`"><p id="`)
		cmmts.WriteString(strconv.Itoa(x.ID))
		cmmts.WriteString(`"><img src="`)
		cmmts.WriteString(esc(URLBuilder(s.Host, "media", "emojitar", x.User.Username, "?type=a")))
		cmmts.WriteString(`" width="30px" height="30px" alt="`)
		cmmts.WriteString(esc(x.User.Username))
		cmmts.WriteString(`"><a href="`)
		cmmts.WriteString(esc(URLBuilder(s.Host, "group_user", "?q=", x.User.Username, "&type=a")))
		cmmts.WriteString(`"><b`)
		cmmts.WriteString(` class="`)
		if x.User.Banned {
			cmmts.WriteString(`banned`)
		}
		if x.Author {
			cmmts.WriteString(`author`)
		}
		cmmts.WriteString(`">`)
		cmmts.WriteString(esc(x.User.Username))
		cmmts.WriteString("</b></a> ")

		if x.Parent > 0 {
			cmmts.WriteString(" ")
			cmmts.WriteString(esc(T(s.Lang, "comments.reply_to")))
			cmmts.WriteString(` <a href="`)
			cmmts.WriteString(esc(s._pth))
			cmmts.WriteString("#")
			cmmts.WriteString(strconv.Itoa(x.Parent))
			cmmts.WriteString(`">`)
			if replied[x.Parent] == "" {
				cmmts.WriteString("???")
			} else {
				cmmts.WriteString(esc(replied[x.Parent]))
			}
			cmmts.WriteString("</a>")
		}
		cmmts.WriteString(" [")
		cmmts.WriteString(x.Posted.UTC().String())
		cmmts.WriteString("]<p>")

		cmmts.WriteString(ParseDescription(s.Host, x.TextContent))
		cmmts.WriteString("<p>👍: ")
		cmmts.WriteString(strconv.Itoa(x.Likes))
		cmmts.WriteString(" ⏩: ")
		cmmts.WriteString(strconv.Itoa(x.Replies))
		cmmts.WriteString("</p></div>\n")
	}
	cmmts.WriteString(s.NavBase(DeviationList{
		Pages: 0,
		More:  c.HasMore,
	}))
	cmmts.WriteString("</details>")
	return cmmts.String()
}

// VisibleDeviation reports whether a deviation may be shown by this instance.
//
// Both the HTML listing and the JSON API ask this, deliberately: an API that
// returned what the pages hide would leak exactly what hide-ai and the NSFW
// setting exist to withhold, and a second copy of the rule is a second thing to
// forget to update.
func VisibleDeviation(d *devianter.Deviation) bool {
	if d.AI && CFG.HideAI {
		return false
	}
	return !d.NSFW || CFG.Nsfw
}

// DeviationList renders devs as an HTML grid, or as an Atom feed when the
// request asked for one and allowAtom permits it. NSFW entries are dropped
// unless the instance allows them. Passing content adds a navigation bar.
func (s skunkyart) DeviationList(devs []devianter.Deviation, allowAtom bool, content ...DeviationList) string {
	if s.Atom && s.Page > 1 {
		s.ReturnHTTPError(400)
		return ""
	}

	atom := allowAtom && s.Atom
	var listContent strings.Builder
	var newest time.Time // the latest entry, for the feed's own <updated>

	for i, l := 0, len(devs); i < l; i++ {
		data := &devs[i]
		if !VisibleDeviation(data) {
			continue
		}
		item := s.listItem(data)
		if atom {
			s.Writer.Header().Add("Content-Type", "application/atom+xml")
			if published := s.writeAtomEntry(&listContent, data, item); published.After(newest) {
				newest = published
			}
		} else {
			s.writeGridEntry(&listContent, data, item)
		}
	}

	if atom {
		feed := s.atomFeed(devs, listContent.String(), newest)
		wr(s.Writer, feed)
		return feed
	}

	var list strings.Builder
	list.WriteString(`<div class="content">`)
	list.WriteString(listContent.String())
	list.WriteString("</div>")
	if content != nil {
		list.WriteString(s.NavBase(content[0]))
	}
	return list.String()
}

// listEntry holds one deviation's values for a list, escaped once: the same
// values go into both the HTML grid and the Atom feed, and html.EscapeString
// produces entities XML accepts too.
type listEntry struct {
	author, title, postURL, preview, fullview string
}

func (s skunkyart) listItem(data *devianter.Deviation) listEntry {
	return listEntry{
		author:   esc(data.Author.Username),
		title:    esc(data.Title),
		postURL:  esc(ConvertDeviantArtURLToSkunkyArt(s.Host, data.Url)),
		preview:  esc(ParseMedia(s.Host, data.Media, 320)),
		fullview: esc(ParseMedia(s.Host, data.Media)),
	}
}

// writeAtomEntry writes one feed entry and returns its published time.
func (s skunkyart) writeAtomEntry(b *strings.Builder, data *devianter.Deviation, item listEntry) time.Time {
	// The entry id is the post's URL here: Atom wants an IRI, and a
	// bare number is not one. Timestamps are RFC 3339, the only form
	// the format allows.
	entryURL := esc(URLBuilder(s.Host, "post", data.Author.Username, "atom-"+strconv.Itoa(data.ID)))
	published := data.PublishedTime.UTC()
	b.WriteString(`<entry><author><name>`)
	b.WriteString(item.author)
	b.WriteString(`</name></author><title>`)
	b.WriteString(item.title)
	b.WriteString(`</title><link rel="alternate" type="text/html" href="`)
	b.WriteString(entryURL)
	b.WriteString(`"/><id>`)
	b.WriteString(entryURL)
	b.WriteString(`</id><published>`)
	b.WriteString(published.Format(time.RFC3339))
	b.WriteString(`</published><updated>`)
	b.WriteString(published.Format(time.RFC3339))
	b.WriteString(`</updated>`)
	b.WriteString(`<media:group><media:title>`)
	b.WriteString(item.title)
	b.WriteString(`</media:title><media:thumbnail url="`)
	b.WriteString(item.preview)
	b.WriteString(`"/></media:group><content type="xhtml"><div xmlns="http://www.w3.org/1999/xhtml"><a href="`)
	b.WriteString(item.postURL)
	b.WriteString(`"><img src="`)
	b.WriteString(item.fullview)
	b.WriteString(`"/></a><p>`)
	b.WriteString(ParseDescription(s.Host, data.TextContent))
	b.WriteString(`</p></div></content></entry>`)
	return published
}

func (s skunkyart) writeGridEntry(b *strings.Builder, data *devianter.Deviation, item listEntry) {
	b.WriteString(`<div class="block">`)
	if item.fullview != "" && item.preview != "" {
		b.WriteString(`<a title="` + esc(T(s.Lang, "deviation.open")) + `" href="`)
		b.WriteString(item.fullview)
		b.WriteString(`"><img loading="lazy" src="`)
		b.WriteString(item.preview)
		b.WriteString(`" width="15%" alt="`)
		b.WriteString(item.author)
		b.WriteString(" - ")
		b.WriteString(item.title)
		b.WriteString(`"></a>`)
	} else {
		b.WriteString("<h1>[ " + esc(T(s.Lang, "list.text")) + " ]</h1>")
	}
	b.WriteString(`<br><a href="`)
	b.WriteString(item.postURL)
	b.WriteString(`">`)
	b.WriteString(item.author)
	b.WriteString(" - ")
	b.WriteString(item.title)

	if data.NSFW {
		b.WriteString(` [<span class="nsfw">NSFW</span>]`)
	}
	if data.AI {
		b.WriteString(" [🤖]")
	}
	if data.DD {
		b.WriteString(` [<span class="dd">DD</span>]`)
	}

	b.WriteString("</a></div>")
}

// atomFeed wraps the entries in an Atom feed for this request.
func (s skunkyart) atomFeed(devs []devianter.Deviation, entries string, newest time.Time) string {
	var list strings.Builder
	list.WriteString(`<?xml version="1.0" encoding="UTF-8"?><feed xmlns:media="http://search.yahoo.com/mrss/" xmlns="http://www.w3.org/2005/Atom">`)

	list.WriteString(`<title>`)
	switch {
	case s.Type == 0:
		list.WriteString("Daily Deviations")
	case s.Type == 'g' && len(devs) != 0:
		list.WriteString(esc(devs[0].Author.Username))
	default:
		list.WriteString("SkunkyArt")
	}
	list.WriteString(`</title>`)

	// Atom requires a feed id and updated time; readers reject a feed
	// without them. The id is the feed's own URL on this instance.
	feedURL := esc(s.Host + s._pth + "?" + s.Args.Encode())
	if newest.IsZero() {
		newest = time.Now().UTC()
	}
	list.WriteString(`<id>`)
	list.WriteString(feedURL)
	list.WriteString(`</id><updated>`)
	list.WriteString(newest.Format(time.RFC3339))
	list.WriteString(`</updated><link rel="self" type="application/atom+xml" href="`)
	list.WriteString(feedURL)
	list.WriteString(`"/>`)

	list.WriteString(`<link rel="alternate" href="`)
	list.WriteString(esc(s.Host))
	list.WriteString(`"/>`)

	list.WriteString(entries)

	list.WriteString("</feed>")
	return list.String()
}

/* DESCRIPTION/COMMENT PARSER */

// emoticonURL maps an e.deviantart.net emoticon image URL to this instance's
// emote route, by the file's base name without its extension, which is what
// devianter.AEmedia takes. Anything else yields "".
func emoticonURL(host, raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host != "e.deviantart.net" {
		return ""
	}
	name := path.Base(u.Path)
	name = strings.TrimSuffix(name, path.Ext(name))
	if name == "" || name == "." || name == "/" {
		return ""
	}
	return URLBuilder(host, "media", "emojitar", name, "?type=e")
}

// text is one styled run within a description: the rendered HTML, the raw source
// it came from, and the offsets it spans in the original block.
type text struct {
	Txt    string
	TxtRaw string
	From   int
	To     int
}

// ParseDescription renders a DeviantArt description into HTML, handling both the
// Draft.js-style JSON payload and the plain HTML markup DeviantArt returns, and
// rewriting embedded links and artwork references to point at this instance.
// host is the request's scheme and host, as taken by URLBuilder.
//
// Three formats arrive: the current editor's document JSON (a "document"
// key), handled by renderDocument; the older Draft.js JSON ("blocks" and
// "entityMap"), handled below and kept for the posts that still carry it; and
// plain HTML markup from before either.
func ParseDescription(host string, dscr devianter.Text) string {
	if doc, ok := parseDocument(dscr.Html.Markup); ok {
		return renderDocument(host, doc)
	}

	description, dl := dscr.Html.Markup, len(dscr.Html.Markup)
	if dl != 0 && description[0] == '{' && description[dl-1] == '}' {
		return renderDraftJS(host, description)
	}
	if dl != 0 {
		return renderMarkupHTML(host, description)
	}
	return ""
}

// tagBuilder wraps content in each tag in turn, innermost first.
func tagBuilder(content string, tags ...string) string {
	for _, tag := range tags {
		var htm strings.Builder
		htm.WriteString("<")
		htm.WriteString(tag)
		htm.WriteString(">")
		htm.WriteString(content)
		htm.WriteString("</")
		htm.WriteString(tag)
		htm.WriteString(">")
		content = htm.String()
	}
	return content
}

type draftBlock struct {
	Text              string `json:"text"`
	Type              string `json:"type"`
	InlineStyleRanges []struct {
		Offset int    `json:"offset"`
		Length int    `json:"length"`
		Style  string `json:"style"`
	} `json:"inlineStyleRanges"`
	EntityRanges []struct {
		Offset int `json:"offset"`
		Length int `json:"length"`
		Key    int `json:"key"`
	} `json:"entityRanges"`
	Data struct {
		TextAlignment string `json:"textAlignment"`
	} `json:"data"`
}

type draftDescription struct {
	Blocks    []draftBlock `json:"blocks"`
	EntityMap map[string]struct {
		Type string `json:"type"`
		Data struct {
			URL    string `json:"url"`
			Config struct {
				// "aligment" is DeviantArt's own spelling; do not correct it.
				Aligment string `json:"aligment"`
				Width    int    `json:"width"`
			} `json:"config"`
			Data devianter.Deviation `json:"data"`
		} `json:"data"`
	} `json:"entityMap"`
}

// renderDraftJS renders the older Draft.js description JSON.
func renderDraftJS(host, description string) string {
	var descr draftDescription
	e := json.Unmarshal([]byte(description), &descr)
	try(e)

	entities := make(map[int]devianter.Deviation)
	urls := make(map[int]string)
	for n, x := range descr.EntityMap {
		num, _ := strconv.Atoi(n)
		if x.Data.URL != "" {
			urls[num] = deleteTrackingFromURL(x.Data.URL)
		}
		entities[num] = x.Data.Data
	}

	var parsedDescription strings.Builder
	for _, x := range descr.Blocks {
		styles := draftStyles(x)
		switch x.Type {
		case "atomic":
			if len(x.EntityRanges) != 0 {
				writeDraftArtwork(&parsedDescription, host, entities[x.EntityRanges[0].Key])
			}
		case "unstyled":
			writeDraftText(&parsedDescription, x, styles, urls)
		}
		parsedDescription.WriteString("<br>")
	}
	return parsedDescription.String()
}

// draftStyles renders each inline style range of a block as tagged text.
func draftStyles(x draftBlock) []text {
	styles := make([]text, len(x.InlineStyleRanges))
	if len(x.InlineStyleRanges) == 0 {
		return styles
	}
	var tags = make(map[int][]string)
	for n, rngs := range x.InlineStyleRanges {
		style := &styles[n]
		switch rngs.Style {
		case "BOLD":
			rngs.Style = "b"
		case "UNDERLINE":
			rngs.Style = "u"
		case "ITALIC":
			rngs.Style = "i"
		}
		style.From = rngs.Offset
		style.To = rngs.Offset + rngs.Length
		ft := style.From * style.To
		tags[ft] = append(tags[ft], rngs.Style)
	}
	for n := range styles {
		style := &styles[n]
		style.TxtRaw = x.Text[style.From:style.To]
		style.Txt = tagBuilder(esc(style.TxtRaw), tags[style.From*style.To]...)
	}
	return styles
}

// writeDraftArtwork writes an embedded deviation as a linked thumbnail.
func writeDraftArtwork(b *strings.Builder, host string, d devianter.Deviation) {
	b.WriteString(`<a href="`)
	b.WriteString(esc(ConvertDeviantArtURLToSkunkyArt(host, d.Url)))
	b.WriteString(`"><img width="50%" src="`)
	b.WriteString(esc(ParseMedia(host, d.Media)))
	b.WriteString(`" alt="`)
	b.WriteString(esc(d.Author.Username))
	b.WriteString(" - ")
	b.WriteString(esc(d.Title))
	b.WriteString(`" title="`)
	b.WriteString(esc(d.Author.Username))
	b.WriteString(" - ")
	b.WriteString(esc(d.Title))
	b.WriteString(`"></a>`)
}

// writeDraftText writes an unstyled block with its styled ranges and link.
func writeDraftText(b *strings.Builder, x draftBlock, styles []text, urls map[int]string) {
	l := len(styles)
	if l == 0 {
		b.WriteString(esc(x.Text))
		return
	}
	for n, r := range styles {
		var tag string
		if x.Type == "header-two" {
			tag = "h2"
		}

		b.WriteString(esc(x.Text[:r.From]))
		if len(urls) != 0 && len(x.EntityRanges) != 0 {
			ra := &x.EntityRanges[0]

			b.WriteString(`<a target="_blank" href="`)
			b.WriteString(esc(urls[ra.Key]))
			b.WriteString(`">`)
			b.WriteString(r.Txt)
			b.WriteString(`</a>`)
		} else if l > n+1 {
			b.WriteString(r.Txt)
		}
		b.WriteString(tagBuilder(tag, esc(x.Text[r.To:])))
	}
}

// renderMarkupHTML renders plain HTML markup, keeping links, DeviantArt
// emoticons, and a few bare formatting tags.
func renderMarkupHTML(host, markup string) string {
	var parsedDescription strings.Builder
	for tt := html.NewTokenizer(strings.NewReader(markup)); ; {
		switch tt.Next() {
		case html.ErrorToken:
			return parsedDescription.String()
		case html.CommentToken, html.DoctypeToken:
			// No renderable content; skip.
		case html.StartTagToken, html.EndTagToken, html.SelfClosingTagToken:
			writeMarkupTag(&parsedDescription, host, tt, tt.Token())
		case html.TextToken:
			parsedDescription.WriteString(esc(string(tt.Text())))
		}
	}
}

func writeMarkupTag(b *strings.Builder, host string, tt *html.Tokenizer, token html.Token) {
	switch token.Data {
	case "a":
		for _, a := range token.Attr {
			if a.Key == "href" {
				url := deleteTrackingFromURL(a.Val)
				b.WriteString(`<a target="_blank" href="`)
				b.WriteString(esc(url))
				b.WriteString(`">`)
				b.WriteString(esc(GetValueOfTag(tt)))
				b.WriteString("</a> ")
			}
		}
	case "img":
		writeEmoticon(b, host, token)
	case "br", "li", "ul", "p", "b":
		// The bare tag, not token.String(): that would carry over
		// whatever attributes DeviantArt's markup put on it.
		if token.Type == html.EndTagToken {
			b.WriteString("</")
		} else {
			b.WriteString("<")
		}
		b.WriteString(token.Data)
		b.WriteString(">")
	case "div":
		b.WriteString("<p> ")
	}
}

// writeEmoticon carries over DeviantArt's emoticons, served through this
// instance; any other image is dropped.
func writeEmoticon(b *strings.Builder, host string, token html.Token) {
	var uri, title string
	for _, a := range token.Attr {
		switch a.Key {
		case "src":
			uri = emoticonURL(host, a.Val)
		case "title":
			title = a.Val
		}
	}
	if uri != "" {
		b.WriteString(`<img src="`)
		b.WriteString(esc(uri))
		b.WriteString(`" alt="`)
		b.WriteString(esc(title))
		b.WriteString(`" title="`)
		b.WriteString(esc(title))
		b.WriteString(`">`)
	}
}
