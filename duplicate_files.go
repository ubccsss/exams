package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"

	zglob "github.com/mattn/go-zglob"
	"github.com/ubccsss/exams/config"
	"github.com/ubccsss/exams/examdb"
)

// findDuplicates returns all duplicates/extra files on disk that don't have a
// corresponding DB entry.
func findDuplicates(w io.Writer, db *examdb.Database) ([]string, error) {
	var duplicate []string

	pattern := path.Join(config.StaticDir, "**/*.pdf*")
	paths, err := zglob.Glob(pattern)
	if err != nil {
		return nil, err
	}

	for _, path := range paths {
		// Strip off config.StaticDir
		staticPath := strings.TrimPrefix(path, config.StaticDir+"/")
		f := db.FindFileByPath(staticPath)
		if f == nil {
			f := examdb.File{
				Path: staticPath,
			}
			if err := f.ComputeHash(); err != nil {
				return nil, err
			}
			f2 := db.FindFile(f.Hash)
			if f2 == nil {
				fmt.Fprintf(w, "file not in DB: %q\n", staticPath)
			} else {
				fmt.Fprintf(w, "%q -> %q\n", staticPath, f2.Path)
				duplicate = append(duplicate, staticPath)
			}
		}
	}
	return duplicate, nil
}

func (s *server) handleListDuplicates(w http.ResponseWriter, r *http.Request) {
	duplicates, err := findDuplicates(w, &db)
	if err != nil {
		handleErr(w, err)
		return
	}
	for _, d := range duplicates {
		fmt.Fprintf(w, "%s\n", d)
	}
	w.Write([]byte("Done."))
}

func (s *server) handleRemoveDuplicates(w http.ResponseWriter, r *http.Request) {
	duplicates, err := findDuplicates(w, &db)
	if err != nil {
		handleErr(w, err)
		return
	}
	for _, d := range duplicates {
		fmt.Fprintf(w, "Removing: %s\n", d)
		p := path.Join(config.StaticDir, d)
		if err := os.Remove(p); err != nil {
			handleErr(w, err)
			return
		}
	}
	w.Write([]byte("Done."))
}

func (s *server) handleListIncorrectLocations(w http.ResponseWriter, r *http.Request) {
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
