package main

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/alecthomas/chroma/v2"
	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/dgunther/mdthing/mermaid"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

// RenderBody converts Markdown to an HTML fragment. usedFallback reports
// whether any mermaid block needed the JS renderer.
func RenderBody(src []byte) ([]byte, bool, error) {
	cr := &codeRenderer{}
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithRendererOptions(
			renderer.WithNodeRenderers(util.Prioritized(cr, 100)),
		),
	)
	var buf bytes.Buffer
	if err := md.Convert(rewriteWikilinks(src), &buf); err != nil {
		return nil, false, err
	}
	return buf.Bytes(), cr.mermaidFallback, nil
}

// RenderPage wraps an HTML body fragment in a complete document: stylesheet,
// the live-reload subscription, and (when includeMermaidJS is set) the
// Mermaid JS bootstrap for diagrams that couldn't render natively.
func RenderPage(body []byte, title string, includeMermaidJS bool) []byte {
	var b bytes.Buffer
	b.WriteString(`<!doctype html><html><head><meta charset="utf-8"><title>`)
	template.HTMLEscape(&b, []byte(title))
	b.WriteString(`</title><link rel="stylesheet" href="/_assets/base.css">`)
	if cfg.CSS != "" {
		b.WriteString(`<link rel="stylesheet" href="/_user.css">`)
	}
	if cfg.Theme == "dark" {
		b.WriteString(`</head><body class="dark">`)
	} else {
		b.WriteString(`</head><body>`)
	}
	b.WriteString(`<article class="markdown-body">`)
	b.Write(body)
	b.WriteString(`</article>`)
	if includeMermaidJS {
		b.WriteString(`<script src="/_assets/mermaid.min.js"></script>`)
		fmt.Fprintf(&b, `<script>mermaid.initialize({startOnLoad:true,theme:'%s'});</script>`, template.JSEscapeString(cfg.MermaidTheme))
	}
	b.WriteString(`<script>new EventSource('/_events').onmessage=function(){location.reload()};</script>`)
	b.WriteString(`</body></html>`)
	return b.Bytes()
}

// RenderStaticPage wraps a body fragment as a self-contained document for
// -html export: styles inlined, mermaid.js inlined only when a diagram fell
// back, no live-reload plumbing.
func RenderStaticPage(body []byte, title string, includeMermaidJS bool) []byte {
	var b bytes.Buffer
	b.WriteString(`<!doctype html><html><head><meta charset="utf-8"><title>`)
	template.HTMLEscape(&b, []byte(title))
	b.WriteString(`</title><style>`)
	css, _ := assetsFS.ReadFile("assets/base.css") // embedded, cannot fail
	b.Write(css)
	if cfg.CSS != "" {
		if user, err := os.ReadFile(cfg.CSS); err == nil {
			b.Write(user)
		}
	}
	b.WriteString(`</style>`)
	if cfg.Theme == "dark" {
		b.WriteString(`</head><body class="dark">`)
	} else {
		b.WriteString(`</head><body>`)
	}
	b.WriteString(`<article class="markdown-body">`)
	b.Write(body)
	b.WriteString(`</article>`)
	if includeMermaidJS {
		js, _ := assetsFS.ReadFile("assets/mermaid.min.js")
		b.WriteString(`<script>`)
		b.Write(js)
		b.WriteString("</script>")
		fmt.Fprintf(&b, `<script>mermaid.initialize({startOnLoad:true,theme:'%s'});</script>`, template.JSEscapeString(cfg.MermaidTheme))
	}
	b.WriteString(`</body></html>`)
	return b.Bytes()
}

// codeRenderer routes ```mermaid fences to a native SVG render (falling back
// to a raw <pre class="mermaid"> for unsupported diagrams) and every other
// fence through chroma syntax highlighting.
type codeRenderer struct {
	mermaidFallback bool
}

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
		if cfg.MermaidRenderer != "js" {
			theme := mermaid.Light
			if cfg.Theme == "dark" {
				theme = mermaid.Dark
			}
			if svg, err := mermaid.Render(code, theme); err == nil {
				w.Write(svg)
				w.WriteString("\n")
				return ast.WalkSkipChildren, nil
			}
		}
		r.mermaidFallback = true
		w.WriteString(`<pre class="mermaid">`)
		template.HTMLEscape(w, code)
		w.WriteString("</pre>\n")
		return ast.WalkSkipChildren, nil
	}
	var buf bytes.Buffer
	style := "github"
	if cfg.Theme == "dark" {
		style = "github-dark"
	}
	if err := highlight(&buf, string(code), lang, style); err != nil {
		w.WriteString("<pre><code>")
		template.HTMLEscape(w, code)
		w.WriteString("</code></pre>\n")
	} else {
		w.Write(buf.Bytes())
	}
	return ast.WalkSkipChildren, nil
}

// highlight writes code as an inline-styled HTML fragment (no standalone
// document, no <style> block — chroma's registered "html" formatter emits
// a full page, which is wrong inside ours).
func highlight(w io.Writer, code, lang, styleName string) error {
	lexer := lexers.Get(lang)
	if lexer == nil {
		return fmt.Errorf("no lexer for %q", lang)
	}
	it, err := chroma.Coalesce(lexer).Tokenise(nil, code)
	if err != nil {
		return err
	}
	return chromahtml.New().Format(w, styles.Get(styleName), it)
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
