package server

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// buildBreadcrumbs constructs the breadcrumbs trail for a given path. The
// resulting Hrefs include indexQuery so that every intermediate link
// preserves the current index-panel state across HTMX-boosted navigation.
//
// Regardless of isFile, intermediate segments link to /browse/ and the
// final segment is marked Current (no link): when isFile is true the
// final segment names the current file, when isFile is false it names
// the current directory.
func buildBreadcrumbs(path string, isFile bool, indexQuery string) BreadcrumbsData {
	data := BreadcrumbsData{
		HomeHref: "/browse/" + indexQuery,
	}
	path = strings.Trim(path, "/")
	if path == "" {
		return data
	}
	segments := strings.Split(path, "/")
	accumulated := ""
	for i, seg := range segments {
		if accumulated == "" {
			accumulated = seg
		} else {
			accumulated += "/" + seg
		}
		isLast := i == len(segments)-1
		if isLast {
			data.Crumbs = append(data.Crumbs, Crumb{Label: seg, Current: true})
			continue
		}
		data.Crumbs = append(data.Crumbs, Crumb{
			Label: seg,
			Href:  "/browse/" + accumulated + indexQuery,
		})
	}
	return data
}

// readDirEntries returns the visible entries of a notes directory as
// IndexEntry values. The Href for each entry already includes indexQuery
// so that clicking a link preserves the index-panel state.
func readDirEntries(absPath, relPath, indexQuery string) ([]IndexEntry, error) {
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
		entryPath := name
		if relPath != "" {
			entryPath = filepath.ToSlash(filepath.Join(relPath, name))
		}
		var href string
		if de.IsDir() {
			href = "/browse/" + entryPath + indexQuery
		} else {
			href = "/view/" + entryPath + indexQuery
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
