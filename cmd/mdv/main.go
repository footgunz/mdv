package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dgunther/mdv/internal/config"
	"github.com/dgunther/mdv/internal/render"
	"github.com/dgunther/mdv/internal/server"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
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

	srv := server.New(filepath.Dir(abs), filepath.Base(abs), rend, cfg.CSS)

	var reloader *server.Reloader
	err = wails.Run(&options.App{
		Title:       filepath.Base(abs),
		Width:       cfg.WindowWidth,
		Height:      cfg.WindowHeight,
		AssetServer: &assetserver.Options{Handler: srv.Handler()},
		OnStartup: func(ctx context.Context) {
			setDockIcon()
			// Ctrl-C is handled by wails itself (SIGINT/SIGTERM -> Quit).
			if cfg.Watch {
				rl, err := server.NewReloader(srv.Current, func() {
					wruntime.EventsEmit(ctx, "mdv:reload")
				})
				if err != nil {
					fmt.Fprintln(os.Stderr, "mdv: live reload disabled:", err)
					return
				}
				reloader = rl
				srv.SetOnNav(func(navAbs string) {
					// best-effort: an unwatchable dir just means no live reload there
					_ = rl.Watch(filepath.Dir(navAbs))
				})
			}
		},
		OnShutdown: func(ctx context.Context) {
			if reloader != nil {
				_ = reloader.Close()
			}
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "mdv:", err)
		os.Exit(1)
	}
}
