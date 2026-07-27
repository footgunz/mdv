package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/dgunther/mdv/internal/config"
	"github.com/dgunther/mdv/internal/render"
	"github.com/dgunther/mdv/internal/server"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// bundled reports whether we are running as mdv.app/Contents/MacOS/mdv, where
// Finder delivers the document as an Apple Event instead of argv and starting
// with no file is legitimate.
func bundled() bool {
	exe, err := os.Executable()
	return err == nil && strings.Contains(exe, ".app/Contents/MacOS/")
}

func main() {
	rendererFlag := flag.String("mermaid-renderer", "", "mermaid renderer: native or js (overrides config)")
	htmlFlag := flag.Bool("html", false, "render self-contained HTML to stdout and exit")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: mdv [-html] [-mermaid-renderer native|js] <file.md>")
		flag.PrintDefaults()
	}
	flag.Parse()

	var abs string
	switch {
	case flag.NArg() == 1:
		var err error
		abs, err = filepath.Abs(flag.Arg(0))
		if err != nil {
			fmt.Fprintln(os.Stderr, "mdv:", err)
			os.Exit(1)
		}
		if info, err := os.Stat(abs); err != nil || info.IsDir() {
			fmt.Fprintf(os.Stderr, "mdv: cannot read %s\n", flag.Arg(0))
			os.Exit(1)
		}
	// A bundle launched from Finder gets its document as an Apple Event after
	// startup (see OnFileOpen below), so no argument is fine there.
	case flag.NArg() == 0 && bundled() && !*htmlFlag:
	default:
		flag.Usage()
		os.Exit(2)
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

	dir, entry, title := "", "", "mdv"
	if abs != "" {
		dir, entry = filepath.Dir(abs), filepath.Base(abs)
		title = entry
	}
	srv := server.New(dir, entry, rend, cfg.CSS)

	var (
		mu       sync.Mutex
		appCtx   context.Context
		reloader *server.Reloader
	)
	// macOS hands "Open With" documents to a running instance as an Apple
	// Event. Wails buffers these, so one arriving before the window exists is
	// picked up by the initial load of "/"; a later one needs a navigation.
	openFile := func(path string) {
		p, err := filepath.Abs(path)
		if err != nil {
			return
		}
		srv.SetEntry(p)
		mu.Lock()
		ctx := appCtx
		mu.Unlock()
		if ctx != nil {
			wruntime.WindowExecJS(ctx, "location.replace('/')")
		}
	}

	if err := wails.Run(&options.App{
		Title:       title,
		Width:       cfg.WindowWidth,
		Height:      cfg.WindowHeight,
		AssetServer: &assetserver.Options{Handler: srv.Handler()},
		Mac:         &mac.Options{OnFileOpen: openFile},
		OnStartup: func(ctx context.Context) {
			mu.Lock()
			appCtx = ctx
			mu.Unlock()
			setDockIcon()
			// Ctrl-C is handled by wails itself (SIGINT/SIGTERM -> Quit).
			if cfg.Watch {
				rl, err := server.NewReloader(srv.Current, func() {
					wruntime.EventsEmit(ctx, "mdv:reload")
				})
				if err != nil {
					fmt.Fprintln(os.Stderr, "mdv: live reload disabled:", err)
				} else {
					reloader = rl
				}
			}
			srv.SetOnNav(func(navAbs string) {
				wruntime.WindowSetTitle(ctx, filepath.Base(navAbs))
				if reloader != nil {
					// best-effort: an unwatchable dir just means no live reload there
					_ = reloader.Watch(filepath.Dir(navAbs))
				}
			})
		},
		OnDomReady: func(ctx context.Context) {
			// An open event that landed after the placeholder was served:
			// nothing else will trigger the navigation.
			if srv.Current() == "" && srv.HasEntry() {
				wruntime.WindowExecJS(ctx, "location.replace('/')")
			}
		},
		OnShutdown: func(ctx context.Context) {
			if reloader != nil {
				_ = reloader.Close()
			}
		},
	}); err != nil {
		fmt.Fprintln(os.Stderr, "mdv:", err)
		os.Exit(1)
	}
}
