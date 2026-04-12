package server

import (
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// viewPath percent-encodes each segment of a relative file/dir path for
// use in /view/ URLs, so names with spaces, #, ? etc. produce valid hrefs.
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

// buildDirBreadcrumbs constructs the sidebar header trail for directory
// mode. Intermediate segments link up the directory chain via /dir/{path}.
// The final segment is marked Current (no link).
func buildDirBreadcrumbs(sidebarDir string) BreadcrumbsData {
	data := BreadcrumbsData{
		Mode: "dir",
	}
	sidebarDir = strings.Trim(sidebarDir, "/")
	if sidebarDir == "" {
		return data
	}
	segments := strings.Split(sidebarDir, "/")
	accumulated := ""
	for i, seg := range segments {
		if accumulated == "" {
			accumulated = seg
		} else {
			accumulated += "/" + seg
		}
		if i == len(segments)-1 {
			data.Crumbs = append(data.Crumbs, Crumb{Label: seg, Current: true})
			continue
		}
		data.Crumbs = append(data.Crumbs, Crumb{
			Label: seg,
			Href:  "/dir/" + viewPath(accumulated),
		})
	}
	return data
}

// buildTagBreadcrumbs constructs the sidebar header trail for a single
// tag view.
func buildTagBreadcrumbs(tag string) BreadcrumbsData {
	return BreadcrumbsData{
		Mode: "tag",
		Crumbs: []Crumb{
			{Label: tag, Current: true},
		},
	}
}

// buildTagsListBreadcrumbs constructs the sidebar header trail for the
// tags index view.
func buildTagsListBreadcrumbs() BreadcrumbsData {
	return BreadcrumbsData{
		Mode: "tags",
	}
}

// readDirEntries returns the visible entries of a notes directory as
// IndexEntry values. Directory entries link to /dir/{path}, file entries
// link to /view/{path}.
func readDirEntries(absPath, relPath string) ([]IndexEntry, error) {
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
		var href string
		if de.IsDir() {
			href = "/dir/" + viewPath(entryRel)
		} else {
			href = "/view/" + viewPath(entryRel)
		}
		entries = append(entries, IndexEntry{
			Name:  name,
			IsDir: de.IsDir(),
			Href:  href,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return entries[i].Name < entries[j].Name
	})
	return entries, nil
}
