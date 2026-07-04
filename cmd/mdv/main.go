package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/dgunther/mdv/internal/config"
	"github.com/dgunther/mdv/internal/render"
	"github.com/dgunther/mdv/internal/server"
	webview "github.com/webview/webview_go"
)

func main() {
	rendererFlag := flag.String("mermaid-renderer", "", "mermaid renderer: native or js (overrides config)")
	htmlFlag := flag.Bool("html", false, "render self-contained HTML to stdout and exit")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: mdv [-html] [-mermaid-renderer native|js] <file.md>")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	abs, err := filepath.Abs(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "mdv:", err)
		os.Exit(1)
	}
	if info, err := os.Stat(abs); err != nil || info.IsDir() {
		fmt.Fprintf(os.Stderr, "mdv: cannot read %s\n", flag.Arg(0))
		os.Exit(1)
	}

	cfg := config.Load()
	switch *rendererFlag {
	case "":
		// defer to config
	case "native", "js":
		cfg.MermaidRenderer = *rendererFlag
	default:
		fmt.Fprintln(os.Stderr, "mdv: -mermaid-renderer must be native or js")
		flag.Usage()
		os.Exit(2)
	}
	rend := render.Renderer{Cfg: cfg}

	if *htmlFlag {
		src, err := os.ReadFile(abs)
		if err != nil {
			fmt.Fprintln(os.Stderr, "mdv:", err)
			os.Exit(1)
		}
		body, fallback, err := rend.Body(src)
		if err != nil {
			fmt.Fprintln(os.Stderr, "mdv:", err)
			os.Exit(1)
		}
		_, _ = os.Stdout.Write(rend.StaticPage(body, filepath.Base(abs), fallback))
		return
	}

	hub := server.NewHub()
	srv := server.New(filepath.Dir(abs), hub, rend, cfg.CSS)

	if cfg.Watch {
		reloader, err := server.NewReloader(srv.Current, hub.Broadcast)
		if err != nil {
			fmt.Fprintln(os.Stderr, "mdv:", err)
			os.Exit(1)
		}
		defer reloader.Close()
		srv.SetOnNav(func(navAbs string) {
			// best-effort: an unwatchable dir just means no live reload there
			_ = reloader.Watch(filepath.Dir(navAbs))
		})
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintln(os.Stderr, "mdv:", err)
		os.Exit(1)
	}
	// No graceful shutdown: WebKit keeps the SSE connection open past window
	// close, so Server.Shutdown would wait on it forever and the process would
	// linger in the dock. Process exit closes every socket anyway.
	httpSrv := &http.Server{Handler: srv.Handler()}
	go func() { _ = httpSrv.Serve(ln) }()

	url := fmt.Sprintf("http://%s/%s", ln.Addr().String(), filepath.Base(abs))

	w := webview.New(false)
	defer w.Destroy()
	setDockIcon()
	w.SetTitle(filepath.Base(abs))
	w.SetSize(cfg.WindowWidth, cfg.WindowHeight, webview.HintNone)

	// Ctrl-C in the launching terminal closes the window cleanly. Terminate
	// touches AppKit, so it must run on the UI thread via Dispatch — calling
	// it straight from this goroutine segfaults.
	sig := make(chan os.Signal, 2)
	signal.Notify(sig, os.Interrupt)
	go func() {
		<-sig
		w.Dispatch(w.Terminate)
		<-sig
		os.Exit(130) // second Ctrl-C: force quit
	}()

	w.Navigate(url)
	w.Run() // blocks until the window is closed
}
