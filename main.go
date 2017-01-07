package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"regexp"
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
	dbFile        = "exams.json"
	templateGlob  = "templates/*"
	classifierDir = "data/classifiers"
)

var (
	yearRegex = regexp.MustCompile("(20|19)\\d{2}")
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
	if err := json.Unmarshal(raw, &db); err != nil {
		return err
	}
	if err := verifyConsistency(); err != nil {
		return err
	}
	return nil
}

func saveDatabase() error {
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
	for _, course := range db.Courses {
		for _, year := range course.Years {
			for _, f := range year.Files {
				if len(f.Hash) == 0 {
					if err := f.ComputeHash(); err != nil {
						return err
					}
				}
			}
		}
	}

	for _, f := range db.PotentialFiles {
		f.ComputeScore(db)
		f.Path = strings.TrimSpace(f.Path)
		f.Source = strings.TrimSpace(f.Source)
	}
	sort.Sort(examdb.FileSlice(db.PotentialFiles))

	return nil
}

func saveAndGenerate() error {
	if err := generator.All(); err != nil {
		return err
	}
	if err := saveDatabase(); err != nil {
		return err
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

func fetchFileAndSave(course string, year int, term, name, href string) error {
	resp, err := http.Get(href)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	base := path.Base(href)
	dir := fmt.Sprintf("%s/%s/%d", examsDir, course, year)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	raw, _ := ioutil.ReadAll(resp.Body)
	file := path.Join(dir, base)
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		return errors.New("file already exists")
	}
	if err := ioutil.WriteFile(file, raw, 0755); err != nil {
		return err
	}
	db.AddFile(course, year, term, name, file, href)
	return nil
}
