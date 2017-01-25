package main

import (
	"encoding/json"
	"html/template"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/goji/httpauth"
	"github.com/pkg/errors"
	"github.com/ubccsss/exams/config"
	"github.com/ubccsss/exams/examdb"
	"github.com/ubccsss/exams/generators"
	"github.com/ubccsss/exams/ml"
	"github.com/urfave/cli"
)

var (
	templates = template.Must(template.ParseGlob(config.TemplateGlob))

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

		if len(db.UnprocessedSources) == 0 {
			if err := saveDatabase(); err != nil {
				log.Printf("%+v", errors.Wrap(err, "err saving database"))
			}
		}
	}
}

func loadDatabase() error {
	raw, err := ioutil.ReadFile(config.DBFile)
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
	if err := ioutil.WriteFile(config.DBFile, raw, 0755); err != nil {
		return err
	}
	return nil
}

var whitespaceRegexp = regexp.MustCompile("  +")

func removeDuplicateWhitespace(str string) string {
	return whitespaceRegexp.ReplaceAllString(strings.TrimSpace(str), " ")
}

var nameToTermMapping = map[string]string{
	"(Term 1)": "W1",
	"(Term 2)": "W2",
	"(Summer)": "S",
}

var nameReplacements = map[string]string{
	"Practice Midterm":  "Sample Midterm",
	"Practice Final":    "Sample Final",
	"Final Sample":      "Sample Final",
	"Midterm 1 Sample":  "Sample Midterm 1",
	"Midterm Sample":    "Sample Midterm",
	"Midterm 2 Sample":  "Sample Midterm 2",
	"Midterm I Sample":  "Sample Midterm 1",
	"Midterm II Sample": "Sample Midterm 2",
	"Practice Quiz":     "Sample Quiz",
}

func verifyConsistency() error {
	log.Println("Verifying consistency of data and doing house keeping...")
	for code, course := range db.Courses {
		for yearnum, year := range course.Years {
			for _, f := range year.Files {
				f.Year = yearnum
				f.Course = code
				for pattern, term := range nameToTermMapping {
					if !strings.Contains(f.Name, pattern) {
						continue
					}

					log.Printf("Fixing %+v", f)
					f.Name = removeDuplicateWhitespace(strings.Replace(f.Name, pattern, "", -1))
					f.Term = term
					log.Printf("Fixed %+v", f)
				}
				for pattern, term := range nameReplacements {
					if !strings.Contains(f.Name, pattern) {
						continue
					}

					log.Printf("Fixing %+v", f)
					f.Name = removeDuplicateWhitespace(strings.Replace(f.Name, pattern, term, -1))
					log.Printf("Fixed %+v", f)
				}
				f.Path = strings.TrimPrefix(f.Path, "static/exams/")
				f.Path = strings.TrimPrefix(f.Path, "static/")
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

		const prefix = "https://www.ugrad.cs.ubc.ca/~q7w9a/exams.cgi/exams.cgi"
		if strings.HasPrefix(f.Source, prefix) {
			stripped := strings.TrimPrefix(f.Source, prefix)
			if url, ok := ugradPathToHTTP(stripped); ok {
				f.Source = url
			}
		}
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
	if err := ml.LoadOrTrainClassifier(&db, config.ClassifierDir); err != nil {
		log.Printf("Failed to load classifier. Classification tasks will not work.: %s", err)
	}

	secureMux := http.NewServeMux()

	secureMux.HandleFunc("/admin/generate", handleGenerate)
	secureMux.HandleFunc("/admin/potential", handlePotentialFileIndex)
	secureMux.HandleFunc("/admin/needfix", handleNeedFixFileIndex)
	secureMux.HandleFunc("/admin/remove404", handleAdminRemove404)
	secureMux.HandleFunc("/admin/file/", handleFile)

	secureMux.HandleFunc("/admin/ml/bayesian/train", handleMLRetrain)
	secureMux.HandleFunc("/admin/ml/google/train", handleMLRetrainGoogle)
	secureMux.HandleFunc("/admin/ml/google/inferpotential", handleMLGoogleInferPotential)
	secureMux.HandleFunc("/admin/ml/google/accuracy", handleMLGoogleAccuracy)

	secureMux.HandleFunc("/admin/ingress/deptcourses", ingressDeptCourses)
	secureMux.HandleFunc("/admin/ingress/deptfiles", ingressDeptFiles)
	secureMux.HandleFunc("/admin/ingress/ubccsss", ingressUBCCSSS)
	secureMux.HandleFunc("/admin/ingress/archive.org", ingressArchiveOrgFiles)

	secureMux.HandleFunc("/admin/", handleAdminIndex)

	username := c.String("user")
	password := c.String("pass")
	if len(password) > 0 {
		http.Handle("/admin/", httpauth.SimpleBasicAuth(username, password)(secureMux))
	} else {
		log.Println("No admin password set, interface disabled.")
	}

	http.HandleFunc("/upload", handleFileUpload)
	http.Handle("/", http.FileServer(http.Dir("static")))

	// Launch 4 source workers
	for i := 0; i < 4; i++ {
		go unprocessedSourceWorker()
	}

	bindAddr := net.JoinHostPort("0.0.0.0", strconv.Itoa(c.Int("port")))
	log.Printf("Listening on %s...", bindAddr)
	return http.ListenAndServe(bindAddr, nil)
}

func main() {
	log.SetFlags(log.Flags() | log.Lshortfile)

	if err := loadDatabase(); err != nil {
		log.Printf("tried to load database: %s", err)
	}

	var err error
	generator, err = generators.MakeGenerator(&db, config.TemplateGlob, config.ExamsDir)
	if err != nil {
		log.Fatal(err)
	}

	app := setupCommands()
	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
