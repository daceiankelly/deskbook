// Command deskbook serves the desk booking web app and API on the office LAN.
//
//	deskbook [serve] -addr 0.0.0.0:8080 -data ./data/deskbook.json -web ./web/dist
//	deskbook verify  <file>                    validate a data or backup file
//	deskbook restore -data <data file> <backup> install a backup (server must be stopped)
//	deskbook recover -data <data file>          install the newest valid backup
//	deskbook set-password -data <data file> <username>   (password from DESKBOOK_NEW_PASSWORD or stdin)
//	deskbook healthcheck [-url http://127.0.0.1:8080/api/health]   exit 0 when healthy (container probes)
//
// Run exactly ONE serve process per data file. See docs/deployment.md.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"deskbook/internal/api"
	"deskbook/internal/auth"
	"deskbook/internal/booking"
	"deskbook/internal/store"
)

const defaultData = "data/deskbook.json"

func main() {
	args := os.Args[1:]
	cmd := "serve"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}
	var err error
	switch cmd {
	case "serve":
		err = serve(args)
	case "verify":
		err = verify(args)
	case "restore":
		err = restore(args)
	case "recover":
		err = recoverLatest(args)
	case "set-password":
		err = setPassword(args)
	case "healthcheck":
		err = healthcheck(args)
	default:
		err = fmt.Errorf("unknown command %q (serve, verify, restore, recover, set-password, healthcheck)", cmd)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", "0.0.0.0:8080", "listen address (LAN interface IP:port)")
	dataPath := fs.String("data", defaultData, "authoritative JSON data file")
	webDir := fs.String("web", "web/dist", "built frontend directory")
	cert := fs.String("tls-cert", "", "TLS certificate (optional, enables HTTPS)")
	key := fs.String("tls-key", "", "TLS private key")
	allowUnsafeEval := fs.Bool("allow-unsafe-eval", false, "add 'unsafe-eval' to the CSP (only if a browser fails to load Pyodide without it)")
	fs.Parse(args)

	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	absWeb, _ := filepath.Abs(*webDir)
	absData, _ := filepath.Abs(*dataPath)
	if rel, err := filepath.Rel(absWeb, absData); err == nil && !strings.HasPrefix(rel, "..") {
		return fmt.Errorf("data file %s must not be inside the web directory %s", absData, absWeb)
	}
	if _, err := os.Stat(filepath.Join(absWeb, "index.html")); err != nil {
		log.Warn("frontend not found; only the API will be served", "web", absWeb)
	}

	st, err := store.Open(absData, store.Options{Log: log, Seed: func() (*store.Document, error) {
		pw := os.Getenv("DESKBOOK_ADMIN_PASSWORD")
		if pw == "" {
			pw = auth.RandomToken()[:20]
			fmt.Printf("\n  Created administrator account\n    username: admin\n    password: %s\n  Change it after first sign-in.\n\n", pw)
		}
		return seedDocument(pw)
	}})
	if err != nil {
		return fmt.Errorf("opening data file: %w", err)
	}
	defer st.Close()

	if n, err := purgePastBookings(st, time.Now()); err != nil {
		return fmt.Errorf("removing past bookings: %w", err)
	} else if n > 0 {
		log.Info("removed bookings from previous days", "count", n)
	}

	apiSrv := api.New(st, log)
	apiSrv.SecureCookies = *cert != ""

	mime.AddExtensionType(".wasm", "application/wasm")
	mime.AddExtensionType(".mjs", "text/javascript")

	mux := http.NewServeMux()
	mux.Handle("/api/", apiSrv.Handler())
	mux.Handle("/", staticHandler(absWeb))

	srv := &http.Server{
		Addr:              *addr,
		Handler:           securityHeaders(mux, *allowUnsafeEval),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errc := make(chan error, 1)
	go func() {
		log.Info("deskbook listening", "addr", *addr, "data", absData, "tls", *cert != "")
		if *cert != "" {
			errc <- srv.ListenAndServeTLS(*cert, *key)
		} else {
			errc <- srv.ListenAndServe()
		}
	}()
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		log.Info("shutting down")
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx) // in-flight writes finish before the store closes
	}
	return nil
}

// purgePastBookings deletes bookings dated before today (office time) when the
// server starts, keeping the data file small. Today's and future bookings are
// kept. The removed bookings remain in earlier backups. Nothing is written when
// there is nothing to remove.
func purgePastBookings(st store.Store, now time.Time) (int, error) {
	if booking.PurgeBeforeToday(st.Snapshot(), now) == 0 {
		return 0, nil
	}
	removed := 0
	_, err := st.Update(func(doc *store.Document) error {
		removed = booking.PurgeBeforeToday(doc, now)
		return nil
	})
	if err != nil {
		return 0, err
	}
	return removed, nil
}

func securityHeaders(next http.Handler, allowUnsafeEval bool) http.Handler {
	// Pyodide compiles WebAssembly, which needs 'wasm-unsafe-eval'. The bundled
	// Pyodide 0.26.4 works without 'unsafe-eval' (verified in Chromium), so it
	// is only added with -allow-unsafe-eval as a fallback for older browsers.
	// connect-src 'self' means the browser itself blocks any non-LAN request.
	script := "script-src 'self' 'wasm-unsafe-eval'"
	if allowUnsafeEval {
		script += " 'unsafe-eval'"
	}
	csp := "default-src 'self'; " + script + "; style-src 'self' 'unsafe-inline'; img-src 'self' data:; " +
		"font-src 'self'; connect-src 'self'; worker-src 'self' blob:; object-src 'none'; base-uri 'none'; " +
		"frame-ancestors 'none'; form-action 'self'"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

// staticHandler serves only files under the built frontend directory. Dotfiles
// are refused; unknown extension-less paths fall back to index.html (SPA).
func staticHandler(root string) http.Handler {
	files := http.FileServer(http.Dir(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		p := path.Clean("/" + r.URL.Path)
		for _, seg := range strings.Split(p, "/") {
			if strings.HasPrefix(seg, ".") {
				http.NotFound(w, r)
				return
			}
		}
		full := filepath.Join(root, filepath.FromSlash(p))
		if fi, err := os.Stat(full); err == nil && !fi.IsDir() {
			switch {
			case strings.HasPrefix(p, "/assets/"):
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			case strings.HasPrefix(p, "/pyodide/"):
				w.Header().Set("Cache-Control", "public, max-age=86400")
			default:
				w.Header().Set("Cache-Control", "no-cache")
			}
			files.ServeHTTP(w, r)
			return
		}
		if ext := path.Ext(p); ext != "" && ext != ".html" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, filepath.Join(root, "index.html"))
	})
}

// healthcheck lets minimal container images (no shell, no curl) probe the server.
func healthcheck(args []string) error {
	fs := flag.NewFlagSet("healthcheck", flag.ExitOnError)
	target := fs.String("url", "http://127.0.0.1:8080/api/health", "health endpoint")
	fs.Parse(args)
	resp, err := (&http.Client{Timeout: 3 * time.Second}).Get(*target)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health endpoint returned %d", resp.StatusCode)
	}
	return nil
}

func dataFlag(name string, args []string) (*flag.FlagSet, *string) {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	p := fs.String("data", defaultData, "authoritative JSON data file")
	fs.Parse(args)
	return fs, p
}

func verify(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: deskbook verify <file>")
	}
	doc, err := store.VerifyFile(args[0])
	if err != nil {
		return err
	}
	active := 0
	for _, b := range doc.Bookings {
		if b.Status == "active" {
			active++
		}
	}
	fmt.Printf("OK  schema=%d revision=%d updated=%s desks=%d employees=%d bookings=%d (active %d)\n",
		doc.SchemaVersion, doc.Revision, doc.UpdatedAt.Format(time.RFC3339), len(doc.Desks), len(doc.Employees), len(doc.Bookings), active)
	return nil
}

func restore(args []string) error {
	fs, data := dataFlag("restore", args)
	if fs.NArg() != 1 {
		return errors.New("usage: deskbook restore -data <data file> <backup file>")
	}
	doc, err := store.Restore(*data, fs.Arg(0))
	if err != nil {
		return err
	}
	fmt.Printf("restored revision %d from %s\n", doc.Revision, fs.Arg(0))
	return nil
}

func recoverLatest(args []string) error {
	_, data := dataFlag("recover", args)
	p, _, err := store.LatestValidBackup(*data)
	if err != nil {
		return err
	}
	doc, err := store.Restore(*data, p)
	if err != nil {
		return err
	}
	fmt.Printf("recovered revision %d from %s\n", doc.Revision, p)
	return nil
}

func setPassword(args []string) error {
	fs, data := dataFlag("set-password", args)
	if fs.NArg() != 1 {
		return errors.New("usage: deskbook set-password -data <data file> <username>")
	}
	pw := os.Getenv("DESKBOOK_NEW_PASSWORD")
	if pw == "" {
		fmt.Fprint(os.Stderr, "new password: ")
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && line == "" {
			return err
		}
		pw = strings.TrimRight(line, "\r\n")
	}
	hash, err := auth.HashPassword(pw)
	if err != nil {
		return err
	}
	st, err := store.Open(*data, store.Options{}) // takes the lock: server must be stopped
	if err != nil {
		return err
	}
	defer st.Close()
	user := strings.ToLower(fs.Arg(0))
	_, err = st.Update(func(doc *store.Document) error {
		for i, e := range doc.Employees {
			if e.Username == user {
				doc.Employees[i].PasswordHash = hash
				doc.Employees[i].Active = true
				return nil
			}
		}
		return fmt.Errorf("no user %q", user)
	})
	if err == nil {
		fmt.Println("password updated")
	}
	return err
}
