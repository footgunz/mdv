package main

import (
	"bytes"
	"html/template"

	"github.com/alecthomas/chroma/v2/quick"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

var md = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithRendererOptions(
		renderer.WithNodeRenderers(util.Prioritized(&codeRenderer{}, 100)),
	),
)

// RenderBody converts Markdown to an HTML fragment.
func RenderBody(src []byte) ([]byte, error) {
	var buf bytes.Buffer
	if err := md.Convert(rewriteWikilinks(src), &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// codeRenderer routes ```mermaid fences to a raw <pre class="mermaid"> and
// every other fence through chroma syntax highlighting.
type codeRenderer struct{}

func (r *codeRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindFencedCodeBlock, r.renderFenced)
	reg.Register(ast.KindCodeBlock, r.renderIndented)
}

func (r *codeRenderer) renderFenced(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.FencedCodeBlock)
	lang := string(n.Language(source))
	code := codeText(node, source)
	if lang == "mermaid" {
		w.WriteString(`<pre class="mermaid">`)
		template.HTMLEscape(w, code)
		w.WriteString("</pre>\n")
		return ast.WalkSkipChildren, nil
	}
	var buf bytes.Buffer
	if err := quick.Highlight(&buf, string(code), lang, "html", "github"); err != nil {
		w.WriteString("<pre><code>")
		template.HTMLEscape(w, code)
		w.WriteString("</code></pre>\n")
	} else {
		w.Write(buf.Bytes())
	}
	return ast.WalkSkipChildren, nil
}

func (r *codeRenderer) renderIndented(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	w.WriteString("<pre><code>")
	template.HTMLEscape(w, codeText(node, source))
	w.WriteString("</code></pre>\n")
	return ast.WalkSkipChildren, nil
}

func codeText(n ast.Node, src []byte) []byte {
	var b bytes.Buffer
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		s := lines.At(i)
		b.Write(s.Value(src))
	}
	return b.Bytes()
}

// replaced by the real implementation in Task 2
func rewriteWikilinks(src []byte) []byte { return src }
