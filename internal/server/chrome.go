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

// buildDirTree builds a tree of directory entries from the root to expandedDir,
// showing the full ancestor chain with proper depth. At each level, all siblings
// are visible but only the ancestor on the path to expandedDir is expanded
// (its children appear indented below it).
//
// For expandedDir="2026/04" the output is:
//
//	depth=0 2026/    (expanded)
//	depth=1   04/    (expanded)
//	depth=2     file.md
//	depth=0 journal/
//	depth=0 README.md
func buildDirTree(root, expandedDir string) ([]IndexEntry, error) {
	expandedDir = strings.Trim(expandedDir, "/")
	if expandedDir == "" {
		return readDirEntriesAtDepth(root, "", 0)
	}

	segments := strings.Split(expandedDir, "/")
	return buildTreeLevel(root, "", segments, 0)
}

// buildTreeLevel recursively builds one level of the tree. It reads entries
// at relPath (depth), and for the entry matching segments[0], marks it
// expanded and recurses into it with segments[1:].
func buildTreeLevel(root, relPath string, segments []string, depth int) ([]IndexEntry, error) {
	entries, err := readDirEntriesAtDepth(root, relPath, depth)
	if err != nil {
		return nil, err
	}
	if len(segments) == 0 {
		return entries, nil
	}

	target := segments[0]
	var result []IndexEntry
	for _, e := range entries {
		if e.IsDir && e.Name == target {
			e.Expanded = true
			result = append(result, e)
			// Recurse into expanded dir
			childRel := e.Name
			if relPath != "" {
				childRel = relPath + "/" + e.Name
			}
			children, err := buildTreeLevel(root, childRel, segments[1:], depth+1)
			if err != nil {
				return nil, err
			}
			result = append(result, children...)
		} else {
			result = append(result, e)
		}
	}
	return result, nil
}

// readDirEntriesAtDepth reads directory entries and sets Depth on each.
func readDirEntriesAtDepth(root, relPath string, depth int) ([]IndexEntry, error) {
	absPath := filepath.Join(root, relPath)
	entries, err := readDirEntries(absPath, relPath)
	if err != nil {
		return nil, err
	}
	for i := range entries {
		entries[i].Depth = depth
	}
	return entries, nil
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
