package app

import (
	"crypto/sha1" //nolint:gosec // G505: SHA-1 is a cache-key hash here, not a security primitive
	"html/template"
	"maps"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/krazywarez/devianter"
	"golang.org/x/net/html"
)

// devianter calls behind variables so tests can count and script them.
var (
	fetchDeviation = devianter.GetDeviation
	fetchComments  = devianter.GetComments
	fetchProfile   = func(name string) (devianter.GRuser, devianter.Error, error) {
		g := devianter.Group{Name: name}
		return g.Get()
	}
	fetchGallery = func(g devianter.Group, page, folderid int) (devianter.Group, devianter.Error, error) {
		return g.Gallery(page, folderid)
	}
	fetchFavourites = func(g devianter.Group, page int, all bool, folderid int) (devianter.Group, devianter.Error) {
		return g.Favourites(page, all, folderid)
	}
)

// groupSearchURL is the DeviantArt group search page for query, paged ten
// results at a time. page counts from 1; 0 means the first page too.
func groupSearchURL(query string, page int) string {
	var url strings.Builder
	url.WriteString("https://www.deviantart.com/groups/?q=")
	url.WriteString(query)
	if page > 1 {
		url.WriteString("&offset=")
		url.WriteString(strconv.Itoa(10 * (page - 1)))
	}
	return url.String()
}

// commentsOrLink renders a comment thread only when the request asked for it
// with ?comments=1, and otherwise a link that does. A thread is a second
// upstream call on every post and profile view, and most viewers never open
// it. total is shown in the link when it is known (0 or more).
func (s skunkyart) commentsOrLink(id, cursor string, kind, total int) template.HTML {
	if s.Args.Get("comments") != "" {
		return template.HTML(s.ParseComments(fetchComments(id, cursor, s.Page, kind))) //nolint:gosec // G203: ParseComments escapes its input
	}

	args := url.Values{}
	maps.Copy(args, s.Args)
	args.Del("p")
	args.Set("comments", "1")

	var link strings.Builder
	link.WriteString(`<p><a href="`)
	link.WriteString(esc(s._pth + "?" + args.Encode()))
	link.WriteString(`">`)
	link.WriteString(esc(T(s.Lang, "deviation.comments")))
	if total >= 0 {
		link.WriteString(" (")
		link.WriteString(strconv.Itoa(total))
		link.WriteString(")")
	}
	link.WriteString("</a></p>")
	return template.HTML(link.String()) //nolint:gosec // G203: escaped above
}

// GRUser renders a group or user page: the about tab, the gallery, or favourites,
// selected by the request's type argument.
func (s skunkyart) GRUser() {
	if len(s.Query) < 1 {
		s.ReturnHTTPError(400)
		return
	}

	var g devianter.Group
	var daError devianter.Error
	g.Name = s.Query
	var err error
	s.Templates.GroupUser.GR, daError, err = fetchProfile(s.Query)
	try(err)
	if daError.RAW != nil {
		s.Error(daError)
		return
	}

	group := &s.Templates.GroupUser

	switch s.Type {
	case 'a':
		s.Atom = false
		for _, x := range group.GR.Gruser.Page.Modules {
			switch x.Name {
			case "about", "group_about":
				s.fillAbout(group, x.ModuleData.GroupAbout, x.ModuleData.About)
			case "cover_deviation":
				group.About.BGMeta = x.ModuleData.CoverDeviation.Deviation
				group.About.BGMeta.Url = ConvertDeviantArtURLToSkunkyArt(s.Host, group.About.BGMeta.Url)
				group.About.BG = ParseMedia(s.Host, group.About.BGMeta.Media)
			case "group_admins":
				var htm strings.Builder
				for _, z := range x.ModuleData.GroupAdmins.Results {
					htm.WriteString(BuildUserPlate(s.Host, z.User.Username))
				}
				group.Admins += template.HTML(htm.String()) //nolint:gosec // G203: BuildUserPlate escapes its input
			}
		}
	case 'g', 'f':
		if s.Page == 0 {
			s.Page++
		}
		if !s.fillGallery(group, g) {
			return
		}
	default:
		s.ReturnHTTPError(400)
	}

	if !s.Atom {
		s.ExecuteTemplate("gruser.htm", "html", &s)
	}
}

// fillAbout fills the about tab from a group's or a user's about module.
func (s skunkyart) fillAbout(group *groupUserPage, groupAbout devianter.GroupAbout, about devianter.About) {
	if group.GR.Owner.Group {
		group.Group = true
		group.CreationDate = groupAbout.FoundatedAt.UTC().String()
		group.About.DescriptionFormatted = template.HTML(ParseDescription(s.Host, groupAbout.Description)) //nolint:gosec // G203: ParseDescription escapes its input
	} else {
		group.About.A = about
		group.CreationDate = time.Unix(time.Now().Unix()-about.RegDate, 0).UTC().String()
		group.About.DescriptionFormatted = template.HTML(ParseDescription(s.Host, about.Description)) //nolint:gosec // G203: ParseDescription escapes its input
		group.About.Social += profileLinks(about)
		group.About.Interests += profileInterests(about)
	}
	group.About.Comments = s.commentsOrLink(strconv.Itoa(group.GR.Gruser.ID), "", 4, -1)
}

func profileLinks(about devianter.About) template.HTML {
	var out template.HTML
	for _, val := range about.SocialLinks {
		var social strings.Builder
		social.WriteString(`<a target="_blank" href="`)
		social.WriteString(esc(val.Value))
		social.WriteString(`">`)
		social.WriteString(esc(val.Value))
		social.WriteString("</a><br>")
		out += template.HTML(social.String()) //nolint:gosec // G203: escaped above
	}
	return out
}

func profileInterests(about devianter.About) template.HTML {
	var out template.HTML
	for _, val := range about.Interests {
		var interest strings.Builder
		interest.WriteString(esc(val.Label))
		interest.WriteString(": <b>")
		interest.WriteString(esc(val.Value))
		interest.WriteString("</b><br>")
		out += template.HTML(interest.String()) //nolint:gosec // G203: escaped above
	}
	return out
}

// fillGallery fills the gallery or favourites tab. It returns false when
// DeviantArt answered with an error, which has already been rendered.
func (s skunkyart) fillGallery(group *groupUserPage, g devianter.Group) bool {
	var err error
	var daError devianter.Error
	var content devianter.Group
	folderid, _ := strconv.Atoi(s.Args.Get("folder"))
	all := s.Args.Get("all") == "true"

	if s.Type == 'f' {
		content, daError = fetchFavourites(g, s.Page, all, folderid)
	} else {
		content, daError, err = fetchGallery(g, s.Page, folderid)
		try(err)
	}

	if daError.RAW != nil {
		s.Error(daError)
		return false
	}

	if folderid > 0 || (s.Type == 'f' && all) {
		group.Gallery.List = template.HTML(s.DeviationList(content.Content.Results, true, DeviationList{ //nolint:gosec // G203: DeviationList escapes its input
			More: content.Content.HasMore,
		}))
		return true
	}
	for _, x := range content.Content.Gruser.Page.Modules {
		if len(x.ModuleData.Folders.Results) != 0 {
			group.Gallery.Folders = s.folderGrid(x.ModuleData.Folders.Results)
		}
		if x.Name == "folder_deviations" {
			group.Gallery.List = template.HTML(s.DeviationList(x.ModuleData.Folder.Deviations, true, DeviationList{ //nolint:gosec // G203: DeviationList escapes its input
				Pages: x.ModuleData.Folder.Pages,
				More:  x.ModuleData.Folder.HasMore,
			}))
		}
	}
	return true
}

// galleryFolder is one entry of a gallery's folder list, as devianter decodes it.
type galleryFolder = struct {
	Deviations int `json:"totalItemCount"`
	FolderId   int //nolint:staticcheck // ST1003: must match devianter's field for the types to be identical
	Size       int
	Name       string
	Thumb      devianter.Deviation
}

// folderGrid renders the non-empty folders of a gallery as thumbnail links.
func (s skunkyart) folderGrid(results []galleryFolder) template.HTML {
	var folders strings.Builder
	folders.WriteString(`<h1 id="folders"><a href="#folders">#</a> ` + esc(T(s.Lang, "gallery.folders")) + `</h1><div class="folders"><br>`)
	for _, x := range results {
		if x.FolderId == -1 || x.Size == 0 {
			continue
		}
		folders.WriteString(`<div class="block folder-item">`)
		if !x.Thumb.NSFW || CFG.Nsfw {
			folders.WriteString(`<a href="`)
			folders.WriteString(esc(ConvertDeviantArtURLToSkunkyArt(s.Host, x.Thumb.Url)))
			folders.WriteString(`"><img loading="lazy" src="`)
			folders.WriteString(esc(ParseMedia(s.Host, x.Thumb.Media)))
			folders.WriteString(`" title="`)
			folders.WriteString(esc(x.Thumb.Title))
			folders.WriteString(`" alt="`)
			folders.WriteString(esc(x.Thumb.Title))
			folders.WriteString(`"></a>`)
		} else {
			folders.WriteString(`<h1>[ <span class="nsfw">NSFW</span> ]</h1>`)
		}
		folders.WriteString("<br>")

		folders.WriteString(`<a href="group_user?folder=`)
		folders.WriteString(strconv.Itoa(x.FolderId))
		folders.WriteString("&q=")
		folders.WriteString(esc(s.Query))
		folders.WriteString("&type=")
		folders.WriteRune(s.Type)
		folders.WriteString(`">`)
		folders.WriteString(esc(x.Name))
		folders.WriteString(`</a>`)

		folders.WriteString("</div>")
	}
	folders.WriteString(`</div><h1 id="content"><a href="#content">#</a> ` + esc(T(s.Lang, "gallery.content")) + `</h1>`)
	return template.HTML(folders.String()) //nolint:gosec // G203: escaped above
}

// groupSearchUsernames scrapes the group names from DeviantArt's group search page.
func groupSearchUsernames(body []byte) []string {
	var usernames []string
	for z := html.NewTokenizer(strings.NewReader(string(body))); ; {
		n, token := z.Next(), z.Token()
		if n == html.ErrorToken {
			return usernames
		}
		if n != html.StartTagToken || token.Data != "a" {
			continue
		}
		for _, x := range token.Attr {
			if x.Key == "class" && x.Val == "u regular username" {
				usernames = append(usernames, GetValueOfTag(z))
			}
		}
	}
}

// groupPlates renders scraped group names as user plates with page navigation.
func (s skunkyart) groupPlates(usernames []string) template.HTML {
	var plates strings.Builder
	plates.WriteString(`<div class="content plates">`)
	for _, name := range usernames {
		plates.WriteString(BuildUserPlate(s.Host, name))
	}
	plates.WriteString(`</div>`)
	plates.WriteString(s.NavBase(DeviationList{
		More: true,
	}))
	return template.HTML(plates.String()) //nolint:gosec // G203: BuildUserPlate escapes its input
}

// Deviation renders a single artwork page, with its description, tags, comments
// and related work. It responds 403 for NSFW posts on instances that disallow them.
func (s skunkyart) Deviation(author, postname string) {
	idSearch := regexp.MustCompile("[0-9]+").FindAllString(postname, -1)
	if len(idSearch) < 1 {
		s.ReturnHTTPError(400)
		return
	}

	var err devianter.Error
	post := &s.Templates.Deviation

	id := idSearch[len(idSearch)-1]
	post.Post, err = fetchDeviation(id, author)
	if err.RAW != nil {
		s.Error(err)
		return
	}

	if post.Post.Deviation.NSFW && !CFG.Nsfw {
		s.Writer.Header().Del("Cache-Control")
		s.Writer.WriteHeader(403)
		wr(s.Writer, `<html><link rel="stylesheet" href="`+
			URLBuilder(s.Host, "stylesheet")+
			`" /><h1>`+esc(T(s.Lang, "error.nsfw"))+`</h1></html>`)
		return
	}

	if post.Post.Comments.Total <= 50 {
		post.Post.Comments.Cursor = ""
	}

	if post.Post.Deviation.TextContent.Excerpt != "" {
		post.Description = template.HTML(ParseDescription(s.Host, post.Post.Deviation.TextContent)) //nolint:gosec // G203: ParseDescription escapes its input
	} else {
		post.Description = template.HTML(ParseDescription(s.Host, post.Post.Deviation.Extended.DescriptionText)) //nolint:gosec // G203: ParseDescription escapes its input
	}

	for _, x := range post.Post.Deviation.Extended.RelatedContent {
		if len(x.Deviations) != 0 {
			post.Related += template.HTML(s.DeviationList(x.Deviations, false)) //nolint:gosec // G203: DeviationList escapes its input
		}
	}

	// hashtags
	for _, x := range post.Post.Deviation.Extended.Tags {
		var tag strings.Builder
		tag.WriteString(` <a href="`)
		tag.WriteString(esc(URLBuilder(s.Host, "search", "?q=", x.Name, "&type=tag")))
		tag.WriteString(`">#`)
		tag.WriteString(esc(x.Name))
		tag.WriteString("</a>")

		post.Tags += template.HTML(tag.String()) //nolint:gosec // G203: escaped above
	}

	post.Comments = s.commentsOrLink(id, post.Post.Comments.Cursor, 1, post.Post.Comments.Total)
	post.StringTime = post.Post.Deviation.PublishedTime.UTC().String()
	post.Post.IMG = ParseMedia(s.Host, post.Post.Deviation.Media)

	s.ExecuteTemplate("deviantion.htm", "html", &s)
}

// DD renders the Daily Deviations page, including each themed strip.
func (s skunkyart) DD() {
	dd, err := devianter.GetDailyDeviations(s.Page)
	if err.RAW != nil {
		s.Error(err)
		return
	}
	var strips strings.Builder
	for _, x := range dd.Strips {
		strips.WriteString(`<h3 class="`)
		strips.WriteString(esc(x.Codename))
		strips.WriteString(`"> <a href="#`)
		strips.WriteString(esc(x.Codename))
		strips.WriteString(`"># </a>`)
		strips.WriteString(esc(x.Title))
		strips.WriteString(`</h3>`)

		strips.WriteString(s.DeviationList(x.Deviations, false))
	}
	s.Templates.DDStrips = template.HTML(strips.String())                                    //nolint:gosec // G203: escaped above
	s.Templates.SomeList = template.HTML(s.DeviationList(dd.Deviations, true, DeviationList{ //nolint:gosec // G203: DeviationList escapes its input
		Pages: 0,
		More:  dd.HasMore,
	}))
	if !s.Atom {
		s.ExecuteTemplate("daily.htm", "html", &s)
	}
}

// Search renders search results for the request's query. Group search is scraped
// rather than fetched from the API, which DeviantArt does not expose to guests.
func (s skunkyart) Search() {
	if s.Query == "" {
		s.ReturnHTTPError(400)
		return
	}

	var err error
	var daError devianter.Error
	ss := &s.Templates.Search
	switch s.Type {
	case 'a', 't':
		ss.Content, daError, err = devianter.PerformSearch(s.Query, s.Page, s.Type)
	case 'g', 'f':
		ss.Content, daError, err = devianter.PerformSearch(s.Query, s.Page, s.Type, s.Args.Get("usr"))
	case 'r': // scraper, since DeviantArt withholds the guest API for group search
		dwnld := Download(groupSearchURL(s.Query, s.Page))
		if usernames := groupSearchUsernames(dwnld.Body); len(usernames) != 0 {
			ss.List = s.groupPlates(usernames)
		}
	default:
		s.ReturnHTTPError(400)
		return
	}
	try(err)

	if s.Type != 'r' {
		if daError.RAW != nil {
			s.Error(daError)
			return
		}

		ss.List = template.HTML(s.DeviationList(ss.Content.Results, false, DeviationList{ //nolint:gosec // G203: DeviationList escapes its input
			Pages: ss.Content.Pages,
			More:  ss.Content.HasMore,
		}))
	}

	s.ExecuteTemplate("search.htm", "html", &s)
}

// fetchAvatar is devianter.AEmedia behind a variable so tests can count calls.
var fetchAvatar = devianter.AEmedia

// Emojitar proxies a user's avatar or emoji image, selected by the request's
// type argument. With the media cache on, the image is served from it after
// the first fetch: avatars are on every comment and listing, and DeviantArt
// answers each fetch with up to three requests.
func (s skunkyart) Emojitar(name string) {
	if name == "" || (s.Type != 'a' && s.Type != 'e') {
		s.ReturnHTTPError(400)
		return
	}

	key := sha1.Sum([]byte("emojitar:" + string(s.Type) + ":" + strings.ToLower(name))) //nolint:gosec // G401: cache key, not a security primitive
	if CFG.Cache.Enabled {
		if body := cachedBody(key); body != nil {
			_, _ = s.Writer.Write(body)
			return
		}
	}

	ae, e := fetchAvatar(name, s.Type)
	if e != nil {
		s.ReturnHTTPError(404)
		return
	}
	if CFG.Cache.Enabled {
		storeBody(key, []byte(ae))
	}
	wr(s.Writer, ae)
}
