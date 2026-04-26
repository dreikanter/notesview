package server

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dreikanter/nview/internal/index"
)

// viewPath percent-encodes each segment of a relative file/dir path for
// use in URLs, so names with spaces, #, ? etc. produce valid hrefs.
func viewPath(relPath string) string {
	segments := strings.Split(relPath, "/")
	for i, s := range segments {
		segments[i] = url.PathEscape(s)
	}
	return strings.Join(segments, "/")
}

// tagPath percent-encodes a tag name for use in /tags/ URLs.
func tagPath(tag string) string {
	return url.PathEscape(tag)
}

// readDirEntries returns the visible entries of a notes directory as
// IndexEntry values. File entries link to /n/{id} when the note is in the
// index; directory entries carry no href since there is no directory handler.
// A non-nil idx populates Type from each .md file's frontmatter.
func readDirEntries(absPath, relPath string, idx *index.NoteIndex) ([]IndexEntry, error) {
	dirEntries, err := os.ReadDir(absPath)
	if err != nil {
		return nil, err
	}
	entries := make([]IndexEntry, 0, len(dirEntries))
	for _, de := range dirEntries {
		name := de.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if !de.IsDir() && !strings.HasSuffix(name, ".md") {
			continue
		}
		entryRel := name
		if relPath != "" {
			entryRel = filepath.ToSlash(filepath.Join(relPath, name))
		}
		entry := IndexEntry{
			Name:  name,
			IsDir: de.IsDir(),
		}
		if !de.IsDir() && idx != nil {
			if ne, ok := idx.NoteEntryByRel(entryRel); ok {
				entry.Type = ne.Type
				if ne.ID > 0 {
					entry.Href = fmt.Sprintf("/n/%d", ne.ID)
				}
			}
		}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return entries[i].Name < entries[j].Name
	})
	return entries, nil
}
