package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/utkarshjoshi/walls/apps/api/internal/ingest"
	"github.com/utkarshjoshi/walls/apps/api/internal/scenecheck"
)

const sceneUsage = `usage: walls scene check <folder>...

Checks live scene folders (scene.html, poster.jpg, meta.json) against the
scene standard, docs/16-live-scene-standard.md. Prints every reason a scene
fails and exits non-zero if any does. Reads files only; the catalog is not
touched. Given a folder of scene folders, it checks each one inside.`

// cmdScene handles "walls scene check". It needs no settings and opens
// neither the catalog nor the object store.
func cmdScene(args []string) error {
	if len(args) == 0 || args[0] != "check" {
		fmt.Fprintln(os.Stderr, sceneUsage)
		return errors.New("expected: walls scene check <folder>...")
	}
	fs := flag.NewFlagSet("scene check", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprintln(fs.Output(), sceneUsage) }
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		fmt.Fprintln(os.Stderr, sceneUsage)
		return errors.New("name at least one scene folder")
	}

	// A folder that is not itself a scene but holds scene folders stands for
	// all of them, so "walls scene check seed/scenes" does what it looks like.
	var dirs []string
	for _, arg := range fs.Args() {
		if inner := sceneFolders(arg); !ingest.IsSceneDir(arg) && len(inner) > 0 {
			dirs = append(dirs, inner...)
			continue
		}
		dirs = append(dirs, arg)
	}

	results := make([]scenecheck.Result, len(dirs))
	nameWidth := 0
	for i, dir := range dirs {
		results[i] = scenecheck.CheckDir(dir)
		nameWidth = max(nameWidth, len(results[i].Name))
	}
	failed := 0
	for _, res := range results {
		if res.OK() {
			fmt.Printf("PASS  %-*s  %q by %s (%s)\n", nameWidth, res.Name, res.Meta.Title, res.Meta.Author, res.Meta.License)
			continue
		}
		failed++
		fmt.Printf("FAIL  %s\n", res.Name)
		width := 0
		for _, p := range res.Problems {
			width = max(width, len(p.Where()))
		}
		for _, p := range res.Problems {
			fmt.Printf("      %-*s  %s\n", width, p.Where(), p.Message)
		}
	}
	fmt.Printf("\n%d of %d %s passed.\n", len(dirs)-failed, len(dirs), plural(len(dirs), "scene"))
	fmt.Println("Passing means a scene cannot reach anything and follows the layout contract. It does not")
	fmt.Println("judge whether a scene is calm, cheap to run or good to look at; a person still checks that.")
	if failed > 0 {
		return fmt.Errorf("%d %s failed the scene check", failed, plural(failed, "scene"))
	}
	return nil
}

// sceneFolders lists the sub-folders of dir that hold a scene.html.
func sceneFolders(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if sub := filepath.Join(dir, e.Name()); e.IsDir() && ingest.IsSceneDir(sub) {
			out = append(out, sub)
		}
	}
	sort.Strings(out)
	return out
}
