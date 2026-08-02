package app

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/krazywarez/devianter"
	"golang.org/x/net/html"
)

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
	s.Templates.GroupUser.GR, daError, err = g.Get()
	try(err)
	if daError.RAW != nil {
		s.Error(daError)
		return
	}

	group := &s.Templates.GroupUser

	switch s.Type {
	case 'a':
		g := group.GR
		s.Atom = false
		for _, x := range g.Gruser.Page.Modules {
			switch x.Name {
			case "about", "group_about":
				if g.Owner.Group {
					var about = &x.ModuleData.GroupAbout
					group.Group = true
					group.CreationDate = x.ModuleData.GroupAbout.FoundatedAt.UTC().String()
					group.About.DescriptionFormatted = ParseDescription(s.Host, about.Description)
				} else if false {
					group.About.A = x.ModuleData.About
					var about = &group.About.A
					group.CreationDate = time.Unix(time.Now().Unix()-x.ModuleData.About.RegDate, 0).UTC().String()
					group.About.DescriptionFormatted = ParseDescription(s.Host, about.Description)

					for _, val := range x.ModuleData.About.SocialLinks {
						var social strings.Builder
						social.WriteString(`<a target="_blank" href="`)
						social.WriteString(val.Value)
						social.WriteString(`">`)
						social.WriteString(val.Value)
						social.WriteString("</a><br>")
						group.About.Social += social.String()
					}

					for _, val := range x.ModuleData.About.Interests {
						var interest strings.Builder
						interest.WriteString(val.Label)
						interest.WriteString(": <b>")
						interest.WriteString(val.Value)
						interest.WriteString("</b><br>")
						group.About.Interests += interest.String()
					}
				}
				group.About.Comments = s.ParseComments(devianter.GetComments(strconv.Itoa(group.GR.Gruser.ID), "", s.Page, 4))

			case "cover_deviation":
				group.About.BGMeta = x.ModuleData.CoverDeviation.Deviation
				group.About.BGMeta.Url = ConvertDeviantArtURLToSkunkyArt(s.Host, group.About.BGMeta.Url)
				group.About.BG = ParseMedia(s.Host, group.About.BGMeta.Media)
			case "group_admins":
				var htm strings.Builder
				for _, z := range x.ModuleData.GroupAdmins.Results {
					htm.WriteString(BuildUserPlate(s.Host, z.User.Username))
				}
				group.Admins += htm.String()
			}

		}
	case 'g', 'f':
		var all bool
		var content devianter.Group

		folderid, _ := strconv.Atoi(s.Args.Get("folder"))

		if a := s.Args.Get("all"); a == "true" {
			all = true
		}

		if s.Page == 0 {
			s.Page++
		}

		if s.Type == 'f' {
			content, daError = g.Favourites(s.Page, all, folderid)
		} else {
			content, daError, err = g.Gallery(s.Page, folderid)
			try(err)
		}

		if daError.RAW != nil {
			s.Error(daError)
			return
		}

		if folderid > 0 || (s.Type == 'f' && all) {
			group.Gallery.List = s.DeviationList(content.Content.Results, true, DeviationList{
				More: content.Content.HasMore,
			})
		} else {
			for _, x := range content.Content.Gruser.Page.Modules {
				if l := len(x.ModuleData.Folders.Results); l != 0 {
					var folders strings.Builder
					folders.WriteString(`<h1 id="folders"><a href="#folder">#</a> Folders</h1><div class="folders"><br>`)
					for _, x := range x.ModuleData.Folders.Results {
						if x.FolderId != -1 && x.Size != 0 {
							folders.WriteString(`<div class="block folder-item">`)

							if !x.Thumb.NSFW || CFG.Nsfw {
								folders.WriteString(`<a href="`)
								folders.WriteString(ConvertDeviantArtURLToSkunkyArt(s.Host, x.Thumb.Url))
								folders.WriteString(`"><img loading="lazy" src="`)
								folders.WriteString(ParseMedia(s.Host, x.Thumb.Media))
								folders.WriteString(`" title="`)
								folders.WriteString(x.Thumb.Title)
								folders.WriteString(`"></a>`)
							} else {
								folders.WriteString(`<h1>[ <span class="nsfw">NSFW</span> ]</h1>`)
							}
							folders.WriteString("<br>")

							folders.WriteString(`<a href="group_user?folder=`)
							folders.WriteString(strconv.Itoa(x.FolderId))
							folders.WriteString("&q=")
							folders.WriteString(s.Query)
							folders.WriteString("&type=")
							folders.WriteRune(s.Type)
							folders.WriteString(`">`)
							folders.WriteString(x.Name)
							folders.WriteString(`</a>`)

							folders.WriteString("</div>")
						}
					}
					folders.WriteString(`</div><h1 id="content"><a href="#content">#</a> Content</h1>`)
					group.Gallery.Folders = folders.String()
				}

				if x.Name == "folder_deviations" {
					group.Gallery.List = s.DeviationList(x.ModuleData.Folder.Deviations, true, DeviationList{
						Pages: x.ModuleData.Folder.Pages,
						More:  x.ModuleData.Folder.HasMore,
					})
				}
			}
		}
	default:
		s.ReturnHTTPError(400)
	}

	if !s.Atom {
		s.ExecuteTemplate("gruser.htm", "html", &s)
	}
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
	post.Post, err = devianter.GetDeviation(id, author)
	if err.RAW != nil {
		s.Error(err)
		return
	}

	if post.Post.Deviation.NSFW && !CFG.Nsfw {
		s.Writer.WriteHeader(403)
		wr(s.Writer, `<html><link rel="stylesheet" href="`+
			URLBuilder(s.Host, "stylesheet")+
			`" /><h1>NSFW content are disabled on this instance.</h1></html>`)
		return
	}

	if post.Post.Comments.Total <= 50 {
		post.Post.Comments.Cursor = ""
	}

	if post.Post.Deviation.TextContent.Excerpt != "" {
		post.Post.Description = ParseDescription(s.Host, post.Post.Deviation.TextContent)
	} else {
		post.Post.Description = ParseDescription(s.Host, post.Post.Deviation.Extended.DescriptionText)
	}

	for _, x := range post.Post.Deviation.Extended.RelatedContent {
		if len(x.Deviations) != 0 {
			post.Related += s.DeviationList(x.Deviations, false)
		}
	}

	// hashtags
	for _, x := range post.Post.Deviation.Extended.Tags {
		var tag strings.Builder
		tag.WriteString(` <a href="`)
		tag.WriteString(URLBuilder(s.Host, "search", "?q=", x.Name, "&type=tag"))
		tag.WriteString(`">#`)
		tag.WriteString(x.Name)
		tag.WriteString("</a>")

		post.Tags += tag.String()
	}

	post.Comments = s.ParseComments(devianter.GetComments(id, post.Post.Comments.Cursor, s.Page, 1))
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
		strips.WriteString(x.Codename)
		strips.WriteString(`"> <a href="#`)
		strips.WriteString(x.Codename)
		strips.WriteString(`"># </a>`)
		strips.WriteString(x.Title)
		strips.WriteString(`</h3>`)

		strips.WriteString(s.DeviationList(x.Deviations, false))
	}
	s.Templates.DDStrips = strips.String()
	s.Templates.SomeList = s.DeviationList(dd.Deviations, true, DeviationList{
		Pages: 0,
		More:  dd.HasMore,
	})
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
		var (
			usernames = make(map[int]string)
			url       strings.Builder
			num       int
		)

		s.Page++

		url.WriteString("https://www.deviantart.com/groups/?q=")
		url.WriteString(s.Query)
		if s.Page > 1 {
			url.WriteString("&offset=")
			url.WriteString(strconv.Itoa(10 * s.Page))
		}

		dwnld := Download(url.String())

		for z := html.NewTokenizer(strings.NewReader(string(dwnld.Body))); ; {
			if n, token := z.Next(), z.Token(); n == html.StartTagToken && token.Data == "a" {
				for _, x := range token.Attr {
					if x.Key == "class" && x.Val == "u regular username" {
						usernames[num] = GetValueOfTag(z)
						num++
					}
				}
			} else if n == 0 {
				break
			} else {
				continue
			}
		}

		if l := len(usernames); l != 0 {
			ss.List += `<div class="content plates">`
			for x := range len(usernames) {
				ss.List += BuildUserPlate(s.Host, usernames[x])
			}
			ss.List += `</div>`
			ss.List += s.NavBase(DeviationList{
				More: true,
			})
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

		ss.List = s.DeviationList(ss.Content.Results, false, DeviationList{
			Pages: ss.Content.Pages,
			More:  ss.Content.HasMore,
		})
	}

	s.ExecuteTemplate("search.htm", "html", &s)
}

// Emojitar proxies a user's avatar or emoji image, selected by the request's
// type argument.
func (s skunkyart) Emojitar(name string) {
	if name == "" || (s.Type != 'a' && s.Type != 'e') {
		s.ReturnHTTPError(400)
		return
	}

	ae, e := devianter.AEmedia(name, s.Type)
	if e != nil {
		s.ReturnHTTPError(404)
	}
	wr(s.Writer, ae)
}
