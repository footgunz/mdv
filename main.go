package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"

	webview "github.com/webview/webview_go"
)

func main() {
	rendererFlag := flag.String("mermaid-renderer", "", "mermaid renderer: native or js (overrides config)")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: mdthing [-mermaid-renderer native|js] <file.md>")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	abs, err := filepath.Abs(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "mdthing:", err)
		os.Exit(1)
	}
	if info, err := os.Stat(abs); err != nil || info.IsDir() {
		fmt.Fprintf(os.Stderr, "mdthing: cannot read %s\n", flag.Arg(0))
		os.Exit(1)
	}

	cfg = LoadConfig()
	switch *rendererFlag {
	case "":
		// defer to config
	case "native", "js":
		cfg.MermaidRenderer = *rendererFlag
	default:
		fmt.Fprintln(os.Stderr, "mdthing: -mermaid-renderer must be native or js")
		flag.Usage()
		os.Exit(2)
	}

	baseDir := filepath.Dir(abs)
	hub := NewHub()
	srv := NewServer(baseDir, hub)

	if cfg.Watch {
		reloader, err := NewReloader(srv.Current, hub.Broadcast)
		if err != nil {
			fmt.Fprintln(os.Stderr, "mdthing:", err)
			os.Exit(1)
		}
		defer reloader.Close()
		srv.SetOnNav(func(navAbs string) {
			reloader.Watch(filepath.Dir(navAbs))
		})
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintln(os.Stderr, "mdthing:", err)
		os.Exit(1)
	}
	// No graceful shutdown: WebKit keeps the SSE connection open past window
	// close, so Server.Shutdown would wait on it forever and the process would
	// linger in the dock. Process exit closes every socket anyway.
	httpSrv := &http.Server{Handler: srv.Handler()}
	go httpSrv.Serve(ln)

	url := fmt.Sprintf("http://%s/%s", ln.Addr().String(), filepath.Base(abs))

	w := webview.New(false)
	defer w.Destroy()
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
