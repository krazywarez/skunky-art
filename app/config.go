package app

import (
	"encoding/json"
	"os"
	"regexp"
	"skunkyart/static"
	"strconv"
	"time"

	"github.com/krazywarez/devianter"
)

// Release carries the build's version and description, set at link time and
// shown by --help and the API.
var Release struct {
	Version     string
	Description string
}

type cacheConfig struct {
	Enabled        bool   `json:"enabled"`
	MemCache       bool   `json:"memcache"`
	Path           string `json:"path"`
	MaxSize        int64  `json:"max-size"`
	Lifetime       string `json:"lifetime"`
	UpdateInterval int64  `json:"update-interval"`
}

type config struct {
	cfg           string
	Listen        string      `json:"listen"`
	URI           string      `json:"uri"`
	Cache         cacheConfig `json:"cache"`
	Proxy         bool        `json:"proxy"`
	Nsfw          bool        `json:"nsfw"`
	HideAI        bool        `json:"hide-ai"`
	UserAgent     string      `json:"user-agent"`
	DownloadProxy string      `json:"download-proxy"`
	StaticPath    string      `json:"static-path"`
}

// CFG is the running instance's configuration, holding the defaults below until
// ExecuteConfig overwrites them from the config file.
var CFG = config{
	cfg:    "config.json",
	Listen: "127.0.0.1:3003",
	URI:    "/",
	Cache: cacheConfig{
		Enabled:        false,
		Path:           "cache",
		UpdateInterval: 1,
	},
	StaticPath: "static",
	UserAgent:  "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/127.0.0.0 Safari/537.36",
	Proxy:      true,
	Nsfw:       true,
}

var lifetimeParsed int64

// checkCacheWritable creates the cache directory if it is missing and confirms
// this process can actually write into it, returning the error that a real cache
// write would hit.
//
// An unwritable cache directory is otherwise a silent cliff: every media request
// still succeeds by re-downloading from the CDN, so the only symptom is one
// "permission denied" line per request and a cache that never fills.
func checkCacheWritable(path string) error {
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	probe := path + "/.skunkyart-write-probe"
	if err := os.WriteFile(probe, nil, 0600); err != nil {
		return err
	}
	return os.Remove(probe)
}

// ExecuteConfig loads the config file into CFG, validates it, and starts the
// cache rotation loop if caching is on. It exits the process on a config that
// cannot be read, that asks for caching without proxying, or that points caching
// at a directory this process cannot write.
func ExecuteConfig() {
	if CFG.cfg != "" {
		f, err := os.ReadFile(CFG.cfg)
		tryWithExitStatus(err, 1)
		tryWithExitStatus(json.Unmarshal(f, &CFG), 1)
		if CFG.Cache.Enabled && !CFG.Proxy {
			exit("Incompatible settings detected: cannot use caching media content without proxy", 1)
		}

		if CFG.Cache.Enabled {
			if err := checkCacheWritable(CFG.Cache.Path); err != nil {
				exit("Cache directory is not writable by this process (uid "+
					strconv.Itoa(os.Getuid())+"): "+err.Error()+
					"\nGrant that uid write access to the directory, or set cache.enabled to false."+
					"\nThe official container image runs as uid 10000, so a bind-mounted cache needs:"+
					"\n  chown -R 10000:10000 <cache dir on the host>", 1)
			}

			if CFG.Cache.Lifetime != "" {
				var duration int64
				day := 24 * time.Hour.Milliseconds()
				numstr := regexp.MustCompile("[0-9]+").FindAllString(CFG.Cache.Lifetime, -1)
				num, _ := strconv.Atoi(numstr[len(numstr)-1])

				switch unit := CFG.Cache.Lifetime[len(CFG.Cache.Lifetime)-1:]; unit {
				case "i":
					duration = time.Minute.Milliseconds()
				case "h":
					duration = time.Hour.Milliseconds()
				case "d":
					duration = day
				case "w":
					duration = day * 7
				case "m":
					duration = day * 30
				case "y":
					duration = day * 360
				default:
					exit("Invalid unit specified: "+unit, 1)
				}

				lifetimeParsed = duration * int64(num)
			}
			// max-size is documented in megabytes. This was 1024^2, which in Go is
			// XOR (1026), not exponentiation — so the cap was ~1000x too small.
			CFG.Cache.MaxSize *= 1024 * 1024
			go InitCacheSystem()
			if CFG.Cache.MemCache {
				go InitMemCacheJanitor()
			}
		}

		About = instanceAbout{
			Proxy:  CFG.Proxy,
			Nsfw:   CFG.Nsfw,
			HideAI: CFG.HideAI,
		}

		static.StaticPath = CFG.StaticPath
		devianter.UserAgent = CFG.UserAgent
	}
}
