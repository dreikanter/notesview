package server

import (
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// dirQuery formats the canonical query suffix that carries the sidebar's
// sticky directory across links. Empty string means "no sticky directory"
// (the URL has no ?dir= at all). When non-empty the path is always
// explicit — callers resolve any default (note's parent directory)
// before constructing the query.
func dirQuery(path string) string {
	if path == "" {
		return ""
	}
	return "?dir=" + url.QueryEscape(path)
}

// dirLinkHref builds an href that repositions the sidebar to a new
// directory while keeping the current note in view (sticky model).
// notePath is the note that should stay visible, or "" for the
// empty-state page where there's no note to keep.
func dirLinkHref(notePath, newDir string) string {
	q := dirQuery(newDir)
	if notePath == "" {
		return "/" + q
	}
	return "/view/" + notePath + q
}

// fileLinkHref builds an href that changes the note while keeping the
// sidebar on the same directory. The other half of the sticky model.
func fileLinkHref(filePath, sidebarDir string) string {
	return "/view/" + filePath + dirQuery(sidebarDir)
}

// buildBreadcrumbs constructs the sidebar header trail. Intermediate
// segments link back up the directory chain via dirLinkHref so a click
// only repositions the sidebar; the note is untouched. The final
// segment is marked Current (no link).
func buildBreadcrumbs(sidebarDir, notePath string) BreadcrumbsData {
	data := BreadcrumbsData{
		HomeHref: dirLinkHref(notePath, ""),
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
			Href:  dirLinkHref(notePath, accumulated),
		})
	}
	return data
}

// readDirEntries returns the visible entries of a notes directory as
// IndexEntry values. Directory entries link through dirLinkHref so the
// note stays put on click; file entries link through fileLinkHref so
// the sidebar stays put on click.
func readDirEntries(absPath, relPath, notePath string) ([]IndexEntry, error) {
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
			href = dirLinkHref(notePath, entryRel)
		} else {
			href = fileLinkHref(entryRel, relPath)
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
