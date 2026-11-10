// Command walls is the catalog service for the Still wallpaper app: an HTTP
// API for the Discover section plus the operator tools that load content.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/utkarshjoshi/walls/apps/api/internal/catalog"
	"github.com/utkarshjoshi/walls/apps/api/internal/config"
	"github.com/utkarshjoshi/walls/apps/api/internal/httpapi"
	"github.com/utkarshjoshi/walls/apps/api/internal/ingest"
	"github.com/utkarshjoshi/walls/apps/api/internal/seed"
	"github.com/utkarshjoshi/walls/apps/api/internal/storage"
)

const usage = `walls — catalog service for the Still wallpaper app

Usage:
  walls serve                  run the HTTP API
  walls seed [-scenes dir] [-stills dir]
                               load the starter artwork, stills, live scenes and packs
  walls ingest <dir>           import a folder of your own images, videos and live scenes
  walls scene check <dir>...   check live scene folders against the scene standard
  walls rm <id>                remove an item and its stored files
  walls render [flags]         write the starter artwork to a folder (for tuning the generator)

Settings come from WALLS_* environment variables, or a .env file in the
current folder. See .env.example.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]

	var err error
	switch cmd {
	case "serve":
		err = cmdServe(args)
	case "seed":
		err = cmdSeed(args)
	case "ingest":
		err = cmdIngest(args)
	case "rm":
		err = cmdRemove(args)
	case "scene":
		err = cmdScene(args)
	case "render":
		err = cmdRender(args)
	case "help", "-h", "--help":
		fmt.Print(usage)
		return
	default:
		fmt.Fprintf(os.Stderr, "walls: unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if errors.Is(err, flag.ErrHelp) {
		return
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "walls:", err)
		os.Exit(1)
	}
}

// env is everything a command needs that depends on configuration.
type env struct {
	cfg      config.Config
	db       *catalog.DB
	store    storage.Store
	pipeline *ingest.Pipeline
}

func (e *env) close() { e.db.Close() }

// open loads configuration and opens the catalog and the object store.
func open() (*env, error) {
	if err := config.LoadDotEnv(".env"); err != nil {
		return nil, err
	}
	cfg, err := config.FromEnv()
	if err != nil {
		return nil, err
	}
	store, err := storage.New(storage.Options{
		Kind:        cfg.Storage,
		LocalDir:    filepath.Join(cfg.DataDir, "objects"),
		S3Endpoint:  cfg.S3Endpoint,
		S3Region:    cfg.S3Region,
		S3Bucket:    cfg.S3Bucket,
		S3AccessKey: cfg.S3AccessKey,
		S3SecretKey: cfg.S3SecretKey,
	})
	if err != nil {
		return nil, err
	}
	db, err := catalog.Open(filepath.Join(cfg.DataDir, "catalog.db"))
	if err != nil {
		return nil, err
	}
	return &env{cfg: cfg, db: db, store: store,
		pipeline: &ingest.Pipeline{DB: db, Store: store, Cap: cfg.StorageCapBytes}}, nil
}

// storageLabel describes where files go without revealing credentials.
func storageLabel(cfg config.Config) string {
	if cfg.Storage == "s3" {
		return fmt.Sprintf("s3 bucket %q at %s", cfg.S3Bucket, cfg.S3Endpoint)
	}
	return "local folder " + filepath.Join(cfg.DataDir, "objects")
}

func signalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	e, err := open()
	if err != nil {
		return err
	}
	defer e.close()

	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	api := &httpapi.Server{
		DB: e.db, Store: e.store, StorageName: e.cfg.Storage, PublicURL: e.cfg.PublicURL,
		CORSOrigins: e.cfg.CORSOrigins, Log: log,
	}
	srv := &http.Server{
		Addr:              e.cfg.Addr,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second, // the file handler extends this for itself
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}

	ctx, stop := signalContext()
	defer stop()
	failed := make(chan error, 1)
	go func() { failed <- srv.ListenAndServe() }()

	items, _ := e.db.CountItems(ctx)
	log.Info("listening", "addr", e.cfg.Addr, "storage", storageLabel(e.cfg), "items", items,
		"corsOrigins", strings.Join(e.cfg.CORSOrigins, ","))
	if items == 0 {
		log.Warn(`the catalog is empty; run "walls seed" to load the starter set`)
	}

	select {
	case err := <-failed:
		return err
	case <-ctx.Done():
	}
	stop() // a second Ctrl-C now kills the process the usual way
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	return nil
}

func cmdSeed(args []string) error {
	fs := flag.NewFlagSet("seed", flag.ContinueOnError)
	scenes := fs.String("scenes", filepath.Join("seed", "scenes"), "folder of live scenes to register")
	stills := fs.String("stills", filepath.Join("seed", "stills"), "folder of still pictures listed in its stills.json")
	if err := fs.Parse(args); err != nil {
		return err
	}
	e, err := open()
	if err != nil {
		return err
	}
	defer e.close()
	ctx, stop := signalContext()
	defer stop()

	fmt.Printf("Seeding into %s (catalog %s)\n", storageLabel(e.cfg), filepath.Join(e.cfg.DataDir, "catalog.db"))
	stats, err := seed.Run(ctx, e.pipeline, seed.Options{
		ScenesDir: *scenes,
		StillsDir: *stills,
		Logf:      func(format string, a ...any) { fmt.Printf(format+"\n", a...) },
	})
	if err != nil {
		return err
	}

	cats, err := e.db.Categories(ctx)
	if err != nil {
		return err
	}
	total, err := e.db.StoredBytes(ctx, "")
	if err != nil {
		return err
	}
	fmt.Printf("\nDone in %v: %d added, %d updated, %d unchanged, %d removed; %d stills, %d live scenes.\n",
		stats.Elapsed.Round(100*time.Millisecond), stats.Added, stats.Updated, stats.Unchanged, stats.Removed, stats.Stills, stats.Scenes)
	if n := len(stats.ScenesSkipped); n > 0 {
		fmt.Printf("%d %s skipped for failing the scene check (run \"walls scene check\" on a folder to see this again):\n", n, plural(n, "scene"))
		for _, s := range stats.ScenesSkipped {
			fmt.Printf("  %s\n", s.Name)
			for _, reason := range s.Reasons {
				fmt.Printf("      %s\n", reason)
			}
		}
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "CATEGORY\tITEMS")
	for _, c := range cats {
		fmt.Fprintf(tw, "%s\t%d\n", c.Name, c.Count)
	}
	tw.Flush()
	fmt.Printf("Catalog now stores %s of its %s budget.\n", ingest.HumanBytes(total), ingest.HumanBytes(e.cfg.StorageCapBytes))
	return nil
}

func cmdIngest(args []string) error {
	fs := flag.NewFlagSet("ingest", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprintln(fs.Output(), "usage: walls ingest <dir>") }
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: walls ingest <dir>")
	}
	e, err := open()
	if err != nil {
		return err
	}
	defer e.close()
	ctx, stop := signalContext()
	defer stop()

	rep, err := e.pipeline.IngestDir(ctx, fs.Arg(0))
	if err != nil {
		return err
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "FILE\tRESULT\tID\tDETAIL")
	for _, r := range rep.Results {
		id := r.ID
		if id == "" {
			id = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", r.File, r.Status, id, r.Detail)
		for _, more := range r.More {
			fmt.Fprintf(tw, "\t\t\t  %s\n", more)
		}
	}
	tw.Flush()
	fmt.Printf("\n%d of %d files are in the catalog.\n", rep.Imported, len(rep.Results))
	if rep.Imported == 0 {
		if len(rep.Results) == 0 {
			return fmt.Errorf("no images, videos or scene folders found in %s", fs.Arg(0))
		}
		return errors.New("nothing could be imported")
	}
	return nil
}

func cmdRemove(args []string) error {
	fs := flag.NewFlagSet("rm", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprintln(fs.Output(), "usage: walls rm <id>") }
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: walls rm <id>")
	}
	e, err := open()
	if err != nil {
		return err
	}
	defer e.close()
	ctx, stop := signalContext()
	defer stop()

	it, err := e.pipeline.Remove(ctx, fs.Arg(0))
	if errors.Is(err, catalog.ErrNotFound) {
		return fmt.Errorf("no item with id %q", fs.Arg(0))
	}
	if err != nil {
		return err
	}
	fmt.Printf("Removed %s (%q, %s) and its stored files.\n", it.ID, it.Title, it.Kind)
	return nil
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}
