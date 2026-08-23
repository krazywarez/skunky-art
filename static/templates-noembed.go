//go:build !embed

package static

import (
	"bytes"
	"io/fs"
	"os"
	"strings"
	"time"
)

// Templates is the in-memory asset filesystem populated by CopyTemplatesToMemory.
var Templates FS

type file struct {
	path    string
	name    string
	content []byte
}

var templateNames = []string{}
var templates = make(map[string][]file)

// StaticPath is the directory assets are read from at startup.
var StaticPath string

// CopyTemplatesToMemory reads every asset under StaticPath into memory. It exits
// the process on failure, since the frontend cannot serve anything without them.
func CopyTemplatesToMemory() {
	baseDir, err := os.ReadDir(StaticPath)
	try(err)

	for _, c := range baseDir {
		if c.IsDir() {
			templateNames = append(templateNames, c.Name())

			var filePath strings.Builder
			filePath.WriteString(StaticPath)
			filePath.WriteString("/")
			filePath.WriteString(c.Name())

			dir, err := os.ReadDir(filePath.String())
			try(err)

			filePath.WriteString("/")
			for _, cd := range dir {
				f, err := os.ReadFile(filePath.String() + cd.Name())
				try(err)
				templates[c.Name()] = append(templates[c.Name()], file{
					content: f,
					name:    cd.Name(),
					path:    c.Name() + "/" + cd.Name(),
				})
			}
		}
	}
}

// FS serves the in-memory assets. It implements the subset of fs.FS that
// template.ParseFS requires.
type FS struct{}

// Open returns the asset stored at name, or an fs.PathError if there is none.
func (FS) Open(name string) (fs.File, error) {
	for i, l := 0, len(templateNames); i < l; i++ {
		for _, x := range templates[templateNames[i]] {
			if x.content != nil && name == x.path {
				return &File{
					name:    x.path,
					content: bytes.NewBuffer(x.content),
				}, nil
			}
		}
	}
	return nil, &fs.PathError{}
}

// Glob returns the paths of every asset in the directory named by pattern's
// first segment, or an fs.PathError if none match.
func (FS) Glob(pattern string) ([]string, error) {
	trimmed := strings.Split(pattern, "/")
	var matches = []string{}
	for x, s := range templates {
		for i, l := 0, len(s); i < l && trimmed[0] == x; i++ {
			s := s[i]
			matches = append(matches, s.path)
		}
	}
	if len(matches) != 0 {
		return matches, nil
	}
	return nil, &fs.PathError{}
}

func try(err error) {
	if err != nil {
		println(err.Error())
		os.Exit(1)
	}
}

// fileInfo is a minimal fs.FileInfo. Assets are held in memory and never stat'd
// for anything but their name, so the remaining fields report fixed values.
//
// Based on https://github.com/psanford/memfs; required for templates.ParseFS to
// work correctly.
type fileInfo struct {
	name string
}

// Name returns the asset's path.
func (fi fileInfo) Name() string {
	return fi.name
}

// Size reports a fixed placeholder size; callers here never use it.
func (fi fileInfo) Size() int64 {
	return 4096
}

// Mode reports no mode bits: in-memory assets have no filesystem permissions.
func (fileInfo) Mode() fs.FileMode {
	return 0
}

// ModTime reports the zero time, as in-memory assets are never modified.
func (fileInfo) ModTime() time.Time {
	return time.Time{}
}

// IsDir always reports false: only files are stored, never directories.
func (fileInfo) IsDir() bool {
	return false
}

// Sys returns nil, as there is no underlying data source.
func (fileInfo) Sys() any {
	return nil
}

// File is a read-once handle to an in-memory asset.
type File struct {
	name    string
	content *bytes.Buffer
	closed  bool
}

// Stat returns the file's fileInfo. It never fails.
func (f *File) Stat() (fs.FileInfo, error) {
	return fileInfo{
		name: f.name,
	}, nil
}

// Read consumes the asset's contents, reporting fs.ErrClosed once closed.
func (f *File) Read(b []byte) (int, error) {
	if f.closed {
		return 0, fs.ErrClosed
	}
	return f.content.Read(b)
}

// Close marks the file closed. Closing twice reports fs.ErrClosed.
func (f *File) Close() error {
	if f.closed {
		return fs.ErrClosed
	}
	f.closed = true
	return nil
}

// LanguageFiles lists the assets under lang/, which is how the i18n loader finds
// catalogues without the FS needing directory listing.
func LanguageFiles() []string {
	var out []string
	for _, x := range templates["lang"] {
		if strings.HasSuffix(x.name, ".json") {
			out = append(out, x.name)
		}
	}
	return out
}
