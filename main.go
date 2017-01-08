package main

import (
	"encoding/json"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/d4l3k/exams/examdb"
	"github.com/d4l3k/exams/generators"
	"github.com/d4l3k/exams/ml"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

const (
	staticDir     = "static"
	examsDir      = "static/exams"
	dbFile        = "data/exams.json"
	templateGlob  = "templates/*"
	classifierDir = "data/classifiers"
)

var (
	templates = template.Must(template.ParseGlob(templateGlob))

	db        examdb.Database
	generator *generators.Generator
)

func unprocessedSourceWorker() {
	for {
		if len(db.UnprocessedSources) == 0 {
			time.Sleep(1 * time.Second)
			continue
		}

		var f *examdb.File
		db.UnprocessedSourcesMu.Lock()
		if len(db.UnprocessedSources) > 0 {
			f = db.UnprocessedSources[len(db.UnprocessedSources)-1]
			db.UnprocessedSources = db.UnprocessedSources[:len(db.UnprocessedSources)-1]
		}
		db.UnprocessedSourcesMu.Unlock()

		if f == nil {
			continue
		}

		if err := f.ComputeHash(); err != nil {
			log.Printf("error processing source: %+v: %s", f, err)
			continue
		}

		db.AddPotentialFiles(os.Stderr, []*examdb.File{f})
		log.Printf("%d remaining unprocessed sources", len(db.UnprocessedSources))
	}
}

func loadDatabase() error {
	raw, err := ioutil.ReadFile(dbFile)
	if err != nil {
		return err
	}

	db.Mu.Lock()
	err = json.Unmarshal(raw, &db)
	db.Mu.Unlock()

	if err != nil {
		return err
	}
	if err := verifyConsistency(); err != nil {
		return err
	}
	return nil
}

func saveDatabase() error {
	db.Mu.RLock()
	defer db.Mu.RUnlock()

	raw, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		return err
	}
	if err := ioutil.WriteFile(dbFile, raw, 0755); err != nil {
		return err
	}
	return nil
}

func verifyConsistency() error {
	log.Println("Verifying consistency of data and doing house keeping...")
	for code, course := range db.Courses {
		for yearnum, year := range course.Years {
			for _, f := range year.Files {
				f.Year = yearnum
				f.Course = code
				if len(f.Hash) == 0 {
					if err := f.ComputeHash(); err != nil {
						return err
					}
				}
			}
		}
	}

	for _, f := range db.PotentialFiles {
		f.ComputeScore(&db)
		f.Path = strings.TrimSpace(f.Path)
		f.Source = strings.TrimSpace(f.Source)
	}

	db.Mu.Lock()
	defer db.Mu.Unlock()
	sort.Sort(examdb.FileSlice(db.PotentialFiles))

	return nil
}

func saveAndGenerate() error {
	if err := generator.All(); err != nil {
		return errors.Wrap(err, "error generating all")
	}
	if err := saveDatabase(); err != nil {
		return errors.Wrap(err, "err saving database")
	}
	return nil
}

func serveSite(c *cli.Context) error {
	if err := ml.LoadOrTrainClassifier(&db, classifierDir); err != nil {
		return err
	}

	http.Handle("/static/", http.FileServer(http.Dir(".")))

	http.HandleFunc("/admin/generate", handleGenerate)
	http.HandleFunc("/admin/potential", handlePotentialFileIndex)
	http.HandleFunc("/admin/needfix", handleNeedFixFileIndex)
	http.HandleFunc("/admin/file/", handleFile)

	http.HandleFunc("/admin/ml/retrain", handleMLRetrain)

	http.HandleFunc("/admin/ingress/deptcourses", ingressDeptCourses)
	http.HandleFunc("/admin/ingress/deptfiles", ingressDeptFiles)
	http.HandleFunc("/admin/ingress/ubccsss", ingressUBCCSSS)
	http.HandleFunc("/admin/ingress/archive.org", ingressArchiveOrgFiles)

	http.HandleFunc("/admin/", handleAdminIndex)

	// Launch 4 source workers
	for i := 0; i < 4; i++ {
		go unprocessedSourceWorker()
	}

	log.Println("Listening...")
	return http.ListenAndServe("0.0.0.0:8080", nil)
}

func main() {
	if err := loadDatabase(); err != nil {
		log.Printf("tried to load database: %s", err)
	}

	var err error
	generator, err = generators.MakeGenerator(&db, templateGlob, examsDir)
	if err != nil {
		log.Fatal(err)
	}

	app := setupCommands()
	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
