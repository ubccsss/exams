package main

import (
	"fmt"
	"net/http"
	"path"
	"path/filepath"
	"strings"

	zglob "github.com/mattn/go-zglob"
	"github.com/ubccsss/exams/config"
	"github.com/ubccsss/exams/examdb"
)

// findDuplicates returns all duplicates/extra files on disk that don't have a
// corresponding DB entry.
func findDuplicates(db *examdb.Database) ([]string, error) {
	var duplicate []string

	pattern := path.Join(config.StaticDir, "**/*.pdf*")
	paths, err := zglob.Glob(pattern)
	if err != nil {
		return nil, err
	}

	for _, path := range paths {
		// Strip off config.StaticDir
		staticPath := filepath.Join(filepath.SplitList(path)[1:]...)
		f := db.FindFileByPath(staticPath)
		if f == nil {
			duplicate = append(duplicate, path)
		}
	}
	return duplicate, nil
}

func handleListDuplicates(w http.ResponseWriter, r *http.Request) {
	duplicates, err := findDuplicates(&db)
	if err != nil {
		handleErr(w, err)
		return
	}
	for _, d := range duplicates {
		fmt.Fprintf(w, "%s\n", d)
	}
	w.Write([]byte("Done."))
}

func handleListIncorrectLocations(w http.ResponseWriter, r *http.Request) {
	for _, f := range db.Files {
		if len(f.Path) == 0 {
			continue
		}
		dir := f.IdealDir()
		if !strings.HasPrefix(f.Path, dir) {
			fmt.Fprintf(w, "%s: %s\n", dir, f.Path)
		}
	}
	w.Write([]byte("Done."))
}
