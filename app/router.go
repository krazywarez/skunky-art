package app

import (
	"io"
	"net/http"
	url "net/url"
	"skunkyart/static"
	"strconv"
	"strings"
	"time"
)

// Router registers the single catch-all handler that dispatches every path, then
// serves until the process exits. It does not return on success.
func Router() {
	parsepath := func(path string) map[int]string {
		if l := len(CFG.URI); len(path) > l {
			path = path[l-1:]
		} else {
			path = "/"
		}

		parsedpath := make(map[int]string)
		for x := 0; true; x++ {
			slash := strings.Index(path, "/") + 1
			content := path[:slash]
			path = path[slash:]
			if slash == 0 {
				parsedpath[x] = path
				break
			}
			parsedpath[x] = content[:slash-1]
		}
		return parsedpath
	}

	next := func(path map[int]string, from int) string {
		var out strings.Builder
		for x, l := from, len(path)-1; x <= l; x++ {
			out.WriteString(path[x])
			if x != l {
				out.WriteString("/")
			}
		}
		return out.String()
	}

	open := func(name string) []byte {
		file, err := static.Templates.Open(name)
		if err != nil {
			try(err)
			return nil
		}
		defer func() { try(file.Close()) }()

		fileReaded, err := io.ReadAll(file)
		if err != nil {
			try(err)
			return nil
		}
		return fileReaded
	}

	// the function that drives everything
	handle := func(w http.ResponseWriter, r *http.Request) {
		path := parsepath(r.URL.Path)

		// Per-request, not a package global: requests arrive concurrently on
		// different hosts and ports (bots hitting a proxy's alternate ports, for
		// one), and a shared global lets one request's host leak into another's
		// rendered URLs. Those URLs then point at a different origin, which this
		// handler's own default-src 'self' CSP blocks.
		host := "http://" + r.Host
		if h := r.Header["X-Forwarded-Proto"]; len(h) != 0 && h[0] == "https" {
			host = "https://" + r.Host
		}

		var skunky = skunkyart{Version: Release.Version, Host: host}
		skunky._pth = r.URL.Path

		skunky.Args = r.URL.Query()
		arg := skunky.Args.Get
		p, _ := strconv.Atoi(arg("p"))

		skunky.Endpoint = path[1]
		skunky.API.main = &skunky
		skunky.Writer = w
		skunky.BasePath = CFG.URI
		skunky.Lang = ResolveLang(r.Header.Get("Accept-Language"))
		skunky.QueryRaw = arg("q")
		skunky.Query = url.QueryEscape(skunky.QueryRaw)
		skunky.Page = p

		if t := arg("type"); len(t) > 0 {
			skunky.Type = rune(t[0])
		}

		if arg("atom") == "true" {
			skunky.Atom = true
		}

		if CFG.Proxy {
			w.Header().Add("Content-Security-Policy", "default-src 'self'; script-src 'none'; style-src 'self' 'unsafe-inline'")
		} else {
			w.Header().Add("Content-Security-Policy", "default-src 'self'; img-src 'self' *.wixmp.com; script-src 'none'; style-src 'self' 'unsafe-inline'")
		}

		w.Header().Add("X-Frame-Options", "DENY")

		switch skunky.Endpoint {
		// main
		case "":
			skunky.ExecuteTemplate("index.htm", "html", &CFG.URI)
		case "about":
			skunky.Templates.About = About
			skunky.ExecuteTemplate("about.htm", "html", &skunky)
		case "post":
			skunky.Deviation(path[2], path[3])
		case "search":
			skunky.Search()
		case "dd":
			skunky.DD()
		case "group_user":
			skunky.GRUser()

		// media
		case "media":
			switch path[2] {
			case "file":
				if a := arg("filename"); a != "" {
					skunky.SetFilename(a)
				}
				skunky.DownloadAndSendMedia(path[3], next(path, 4))
			case "emojitar":
				skunky.Emojitar(path[3])
			}
		case "stylesheet":
			w.Header().Add("Content-Type", "text/css")
			_, _ = w.Write(open("css/skunky.css"))
			// "auto" is the stylesheet as written: dark, with a light palette
			// behind prefers-color-scheme. Forcing a theme means re-declaring
			// that palette unconditionally, which outranks the media query
			// because it comes later with equal specificity.
			if css := forcedThemeCSS(); css != "" {
				_, _ = w.Write([]byte(css))
			}
		case "favicon.ico":
			_, _ = w.Write(open("images/logo.png"))

		// API
		case "api":
			w.Header().Add("Content-Type", "application/json")
			switch path[2] {
			case "instance":
				skunky.API.Info()
			case "random":
				skunky.API.Random()
			case "search":
				skunky.API.Search()
			case "post":
				skunky.API.Post(path[3], path[4])
			default:
				skunky.API.Error("Not Found", 404)
			}

		// 404
		default:
			skunky.ReturnHTTPError(404)
		}
	}

	http.HandleFunc("/", handle)
	println("SkunkyArt is listening on", CFG.Listen)

	// Explicit timeouts: the bare http.ListenAndServe has none, so a slow client
	// can hold a connection (and its handler) open indefinitely. WriteTimeout is
	// generous because media proxying streams large files through a handler.
	srv := &http.Server{
		Addr:              CFG.Listen,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	tryWithExitStatus(srv.ListenAndServe(), 1)
}
