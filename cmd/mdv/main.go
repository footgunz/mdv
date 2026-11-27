package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/dgunther/mdv/internal/config"
	"github.com/dgunther/mdv/internal/render"
	"github.com/dgunther/mdv/internal/server"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func main() {
	// Launched from Finder (.app bundle) or Explorer (we own our console)
	// rather than a terminal. Decided once, up front: on Windows the answer
	// changes once detachConsole has run.
	gui := guiLaunch()
	// fail reports a fatal error; in a GUI launch nobody can see stderr.
	fail := func(code int, msg string) {
		fmt.Fprint(os.Stderr, msg)
		if gui {
			showError(msg)
		}
		os.Exit(code)
	}

	rendererFlag := flag.String("mermaid-renderer", "", "mermaid renderer: native or js (overrides config)")
	htmlFlag := flag.Bool("html", false, "render self-contained HTML to stdout and exit")
	flag.Usage = func() {
		var b strings.Builder
		fmt.Fprintln(&b, "usage: mdv [-html] [-mermaid-renderer native|js] <file.md>")
		flag.CommandLine.SetOutput(&b)
		flag.PrintDefaults()
		flag.CommandLine.SetOutput(nil) // back to stderr
		fmt.Fprint(os.Stderr, b.String())
		if gui {
			showError(b.String())
		}
	}
	flag.Parse()

	var abs string
	switch {
	case flag.NArg() == 1:
		var err error
		abs, err = filepath.Abs(flag.Arg(0))
		if err != nil {
			fail(1, fmt.Sprintln("mdv:", err))
		}
		if info, err := os.Stat(abs); err != nil || info.IsDir() {
			fail(1, fmt.Sprintf("mdv: cannot read %s\n", flag.Arg(0)))
		}
	// A bundle launched from Finder gets its document as an Apple Event after
	// startup (see OnFileOpen below), so no argument is fine there. On
	// Windows a double-clicked mdv.exe offers an open-file dialog instead.
	case flag.NArg() == 0 && gui && !*htmlFlag:
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

	// Windows: Explorer launches share one window, like Finder's "Open With"
	// on macOS. A later Explorer launch finds Wails' named mutex, forwards its
	// argv here and exits. Terminal launches never take the lock, so they
	// always get their own window and never hijack (or get hijacked by) one
	// opened from Explorer — mirroring the bundle vs. CLI split on macOS.
	var single *options.SingleInstanceLock
	if gui && runtime.GOOS == "windows" {
		single = &options.SingleInstanceLock{
			UniqueId: "mdv-explorer",
			OnSecondInstanceLaunch: func(d options.SecondInstanceData) {
				if p := docArg(d.Args); p != "" {
					if !filepath.IsAbs(p) {
						p = filepath.Join(d.WorkingDirectory, p)
					}
					openFile(p)
				}
				mu.Lock()
				ctx := appCtx
				mu.Unlock()
				if ctx != nil {
					raiseWindow(ctx)
				}
			},
		}
	}

	var pickOnce sync.Once
	if gui {
		detachConsole()
	}
	if err := wails.Run(&options.App{
		Title:              title,
		Width:              cfg.WindowWidth,
		Height:             cfg.WindowHeight,
		AssetServer:        &assetserver.Options{Handler: srv.Handler()},
		Mac:                &mac.Options{OnFileOpen: openFile},
		Windows:            &windows.Options{Theme: windows.SystemDefault},
		SingleInstanceLock: single,
		OnStartup: func(ctx context.Context) {
			mu.Lock()
			appCtx = ctx
			mu.Unlock()
			setDockIcon()
			// Ctrl-C is handled by wails itself (SIGINT/SIGTERM -> Quit), on
			// Windows too: Go maps console Ctrl-C/Ctrl-Break to os.Interrupt.
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
			// Windows GUI launch with no document: ask for one, once. Runs on
			// a goroutine (wails calls OnDomReady with go), so blocking is fine.
			// Cancel leaves the placeholder up.
			if openDialogOnEmpty && !srv.HasEntry() {
				pickOnce.Do(func() {
					p, err := wruntime.OpenFileDialog(ctx, wruntime.OpenDialogOptions{
						Title: "Open Markdown file",
						Filters: []wruntime.FileFilter{
							{DisplayName: "Markdown (*.md, *.markdown)", Pattern: "*.md;*.markdown;*.mdown;*.mkd"},
							{DisplayName: "All files (*.*)", Pattern: "*.*"},
						},
					})
					if err == nil && p != "" {
						openFile(p)
					}
				})
			}
		},
		OnShutdown: func(ctx context.Context) {
			if reloader != nil {
				_ = reloader.Close()
			}
		},
	}); err != nil {
		fail(1, fmt.Sprintln("mdv:", err))
	}
}

// docArg extracts the document from a forwarded second-instance argv, using
// the same flags as main so "-mermaid-renderer js file.md" still finds it.
func docArg(args []string) string {
	fs := flag.NewFlagSet("mdv", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.String("mermaid-renderer", "", "")
	fs.Bool("html", false, "")
	if fs.Parse(args) != nil || fs.NArg() != 1 {
		return ""
	}
	return fs.Arg(0)
}

// raiseWindow brings the shared window forward for a forwarded open. Windows
// won't let a background process take focus outright; toggling topmost
// still lifts it above the Explorer window that sent the file.
func raiseWindow(ctx context.Context) {
	wruntime.WindowUnminimise(ctx)
	wruntime.WindowShow(ctx)
	wruntime.WindowSetAlwaysOnTop(ctx, true)
	wruntime.WindowSetAlwaysOnTop(ctx, false)
}
