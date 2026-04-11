package server

import (
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// buildBreadcrumbs constructs the breadcrumbs trail for a given path.
// Regardless of isFile, intermediate segments link to /browse/ and the
// final segment is marked Current (no link): when isFile is true the
// final segment names the current file, when isFile is false it names
// the current directory.
func buildBreadcrumbs(path string, isFile bool) []Crumb {
	path = strings.Trim(path, "/")
	if path == "" {
		return nil
	}
	segments := strings.Split(path, "/")
	var crumbs []Crumb
	accumulated := ""
	for i, seg := range segments {
		if accumulated == "" {
			accumulated = seg
		} else {
			accumulated += "/" + seg
		}
		isLast := i == len(segments)-1
		if isLast && isFile {
			crumbs = append(crumbs, Crumb{Label: seg, Current: true})
			continue
		}
		if isLast && !isFile {
			crumbs = append(crumbs, Crumb{Label: seg, Current: true})
			continue
		}
		crumbs = append(crumbs, Crumb{
			Label: seg,
			Href:  "/browse/" + accumulated,
		})
	}
	return crumbs
}

// buildSidebarTree walks the notes root and produces the nested tree used
// by the sidebar template. It includes all markdown files and directories,
// skipping dotfiles.
func buildSidebarTree(root string) []SidebarNode {
	return readSidebarDir(root, "")
}

func readSidebarDir(root, rel string) []SidebarNode {
	absPath := filepath.Join(root, rel)
	dirEntries, err := os.ReadDir(absPath)
	if err != nil {
		// Log once and degrade gracefully: a read failure in one subtree
		// (permissions, broken symlink, etc.) shouldn't break the whole
		// sidebar, but it also shouldn't be completely invisible.
		log.Printf("notesview: sidebar: read %q: %v", absPath, err)
		return nil
	}

	var nodes []SidebarNode
	for _, de := range dirEntries {
		name := de.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if !de.IsDir() && !strings.HasSuffix(name, ".md") {
			continue
		}
		entryRel := name
		if rel != "" {
			entryRel = rel + "/" + name
		}
		node := SidebarNode{
			Name:  name,
			Path:  entryRel,
			IsDir: de.IsDir(),
		}
		if de.IsDir() {
			node.Children = readSidebarDir(root, entryRel)
		}
		nodes = append(nodes, node)
	}

	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].IsDir != nodes[j].IsDir {
			return nodes[i].IsDir
		}
		return nodes[i].Name < nodes[j].Name
	})
	return nodes
}
