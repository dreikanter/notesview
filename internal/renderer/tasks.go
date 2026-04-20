package renderer

import (
	"regexp"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

const (
	svgTaskUnchecked = `<svg class="task-unchecked" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect width="18" height="18" x="3" y="3" rx="2"/></svg>`
	svgTaskChecked   = `<svg class="task-checked" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect width="18" height="18" x="3" y="3" rx="2"/><path d="m9 12 2 2 4-4"/></svg>`
)

// TaskCheckBoxExtension replaces goldmark's default <input type="checkbox">
// output for GFM task items with inline Lucide SVG icons.
var TaskCheckBoxExtension goldmark.Extender = &taskCheckBoxExtension{}

type taskCheckBoxExtension struct{}

func (e *taskCheckBoxExtension) Extend(m goldmark.Markdown) {
	m.Renderer().AddOptions(
		renderer.WithNodeRenderers(
			util.Prioritized(&taskCheckBoxRenderer{}, 100),
		),
	)
}

type taskCheckBoxRenderer struct{}

func (r *taskCheckBoxRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(east.KindTaskCheckBox, r.renderTaskCheckBox)
}

func (r *taskCheckBoxRenderer) renderTaskCheckBox(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	if node.(*east.TaskCheckBox).IsChecked {
		_, _ = w.WriteString(svgTaskChecked)
	} else {
		_, _ = w.WriteString(svgTaskUnchecked)
	}
	return ast.WalkContinue, nil
}

var dailyPattern = regexp.MustCompile(`\[daily\]\s*`)

func processTaskSyntax(html string) string {
	return dailyPattern.ReplaceAllString(html, `<span class="task-tag">daily</span> `)
}
