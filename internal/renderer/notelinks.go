package renderer

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/alex/notesview/internal/index"
)

var noteProtoRe = regexp.MustCompile(`href="note://(\d{8}_\d+)"`)
var relativeMdRe = regexp.MustCompile(`href="([^"]+\.md)"`)
var uidInTextRe = regexp.MustCompile(`\b(\d{8}_\d{4,})\b`)

func processNoteLinks(html string, idx *index.Index, currentDir string) string {
	// 1. Resolve note:// protocol links
	html = noteProtoRe.ReplaceAllStringFunc(html, func(match string) string {
		sub := noteProtoRe.FindStringSubmatch(match)
		if sub == nil {
			return match
		}
		uid := sub[1]
		if relPath, ok := idx.Lookup(uid); ok {
			return fmt.Sprintf(`href="/view/%s"`, relPath)
		}
		return fmt.Sprintf(`href="#" class="broken-link" title="Note %s not found"`, uid)
	})

	// 2. Rewrite relative .md links to /view/ routes
	html = relativeMdRe.ReplaceAllStringFunc(html, func(match string) string {
		sub := relativeMdRe.FindStringSubmatch(match)
		if sub == nil {
			return match
		}
		relLink := sub[1]
		// Skip absolute URLs (http://, https://, etc.) and already-rooted paths
		if strings.Contains(relLink, "://") || strings.HasPrefix(relLink, "/") {
			return match
		}
		resolved := path.Clean(path.Join(currentDir, relLink))
		resolved = strings.TrimPrefix(resolved, "/")
		return fmt.Sprintf(`href="/view/%s"`, resolved)
	})

	// 3. Auto-link bare UIDs in text content (not inside HTML tags)
	parts := splitByTags(html)
	for i, part := range parts {
		if !strings.HasPrefix(part, "<") {
			parts[i] = uidInTextRe.ReplaceAllStringFunc(part, func(match string) string {
				if relPath, ok := idx.Lookup(match); ok {
					return fmt.Sprintf(`<a href="/view/%s" class="uid-link">%s</a>`, relPath, match)
				}
				return match
			})
		}
	}
	return strings.Join(parts, "")
}

func splitByTags(html string) []string {
	var parts []string
	for len(html) > 0 {
		tagStart := strings.Index(html, "<")
		if tagStart == -1 {
			parts = append(parts, html)
			break
		}
		if tagStart > 0 {
			parts = append(parts, html[:tagStart])
		}
		tagEnd := strings.Index(html[tagStart:], ">")
		if tagEnd == -1 {
			parts = append(parts, html[tagStart:])
			break
		}
		parts = append(parts, html[tagStart:tagStart+tagEnd+1])
		html = html[tagStart+tagEnd+1:]
	}
	return parts
}
