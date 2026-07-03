package main

import (
	"bytes"
	"html/template"
	"path/filepath"
	"regexp"
	"strings"

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

// RenderPage wraps an HTML body fragment in a complete document: stylesheet,
// Mermaid bootstrap, and the live-reload subscription.
func RenderPage(body []byte, title string) []byte {
	var b bytes.Buffer
	b.WriteString(`<!doctype html><html><head><meta charset="utf-8"><title>`)
	template.HTMLEscape(&b, []byte(title))
	b.WriteString(`</title><link rel="stylesheet" href="/_assets/base.css"></head><body>`)
	b.WriteString(`<article class="markdown-body">`)
	b.Write(body)
	b.WriteString(`</article>`)
	b.WriteString(`<script src="/_assets/mermaid.min.js"></script>`)
	b.WriteString(`<script>mermaid.initialize({startOnLoad:true});</script>`)
	b.WriteString(`<script>new EventSource('/_events').onmessage=function(){location.reload()};</script>`)
	b.WriteString(`</body></html>`)
	return b.Bytes()
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

var wikilinkRe = regexp.MustCompile(`\[\[([^\]|]+)(?:\|([^\]]+))?\]\]`)

// rewriteWikilinks converts Obsidian-style [[target]] and [[target|label]]
// into standard Markdown links. The destination is wrapped in <...> so paths
// with spaces stay valid; ".md" is appended when the target has no extension.
// ponytail: regex prepass, can match inside code spans; upgrade to an AST
// transform only if that becomes a real problem.
func rewriteWikilinks(src []byte) []byte {
	return wikilinkRe.ReplaceAllFunc(src, func(m []byte) []byte {
		g := wikilinkRe.FindSubmatch(m)
		target := strings.TrimSpace(string(g[1]))
		label := target
		if len(g[2]) > 0 {
			label = strings.TrimSpace(string(g[2]))
		}
		href := target
		if filepath.Ext(href) == "" {
			href += ".md"
		}
		return []byte("[" + label + "](<" + href + ">)")
	})
}
