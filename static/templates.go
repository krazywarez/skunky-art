//go:build embed

package static

import (
	"embed"
	"io/fs"
	"strings"
)

// Templates is the asset filesystem compiled into the binary.
//
//go:embed *
var Templates embed.FS

// Enabled reports that assets are embedded in this build.
var Enabled bool = true

// StaticPath is accepted for parity with the non-embed build, where it names the
// directory assets are read from. It is ignored here.
var StaticPath string

// CopyTemplatesToMemory is a no-op in this build: the assets are already embedded.
func CopyTemplatesToMemory() {
	_ = StaticPath
}

// LanguageFiles lists the assets under lang/, which is how the i18n loader finds
// catalogues without the FS needing directory listing.
func LanguageFiles() []string {
	entries, err := fs.ReadDir(Templates, "lang")
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			out = append(out, e.Name())
		}
	}
	return out
}
