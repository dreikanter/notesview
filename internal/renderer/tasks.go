package renderer

import (
	"regexp"
	"strings"
)

const (
	svgTaskUnchecked = `<svg class="task-unchecked" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect width="18" height="18" x="3" y="3" rx="2"/></svg>`
	svgTaskChecked   = `<svg class="task-checked" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect width="18" height="18" x="3" y="3" rx="2"/><path d="m9 12 2 2 4-4"/></svg>`
)

// GFM converts [ ] and [x] to checkbox inputs; we replace those rendered forms.
// [+] is not a GFM task marker so it passes through as literal text.
var taskPatterns = []struct {
	marker  string
	replace string
}{
	{`<input disabled="" type="checkbox"> `, svgTaskUnchecked + ` `},
	{`<input checked="" disabled="" type="checkbox"> `, svgTaskChecked + ` `},
	{"[+] ", svgTaskChecked + ` `},
}

var dailyPattern = regexp.MustCompile(`\[daily\]\s*`)

func processTaskSyntax(html string) string {
	for _, p := range taskPatterns {
		html = strings.ReplaceAll(html, p.marker, p.replace)
	}
	html = dailyPattern.ReplaceAllString(html, `<span class="task-tag">daily</span> `)
	return html
}
