package main

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/goji/httpauth"
	"github.com/pkg/errors"
	"github.com/ubccsss/exams/config"
	"github.com/ubccsss/exams/db"
	"github.com/ubccsss/exams/exambot"
	"github.com/ubccsss/exams/examdb"
	"github.com/ubccsss/exams/generators"
	"github.com/ubccsss/exams/ml"
	"github.com/ubccsss/exams/workers"
	"github.com/urfave/cli"
)

var (
	templates = generators.Templates

	oldDB     examdb.Database
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
	return nil
}

func saveDatabase() error {
	db.Mu.RLock()
	defer db.Mu.RUnlock()

	start := time.Now()
	raw, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		return err
	}
	if err := ioutil.WriteFile(config.DBFile, raw, 0755); err != nil {
		return err
	}
	log.Printf("Saved database in %s.", time.Since(start))
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
	/*
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

					f.HandClassified = true
				}
				db.Files = append(db.Files, year.Files...)
				year.Files = nil
			}
			course.Years = nil
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

		db.Files = append(db.Files, db.PotentialFiles...)
		db.PotentialFiles = nil

		db.Mu.Lock()
		defer db.Mu.Unlock()
		sort.Sort(examdb.FileSlice(db.PotentialFiles))
	*/

	fetched := struct {
		sync.Mutex
		count int
	}{}

	fileChan := make(chan *examdb.File, workers.Count)
	var wg sync.WaitGroup
	for i := 0; i < workers.Count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for f := range fileChan {
				if err := db.FetchFileAndSave(f); err != nil {
					log.Println(err)
				}

				fetched.Lock()
				fetched.count++
				count := fetched.count
				fetched.Unlock()

				if count%100 == 0 {
					log.Printf("Fetched %d", count)
					if err := saveDatabase(); err != nil {
						log.Println(err)
					}
				}
			}
		}()
	}

	for _, f := range db.Files {
		if len(f.Path) != 0 {
			continue
		}

		if !(f.LastResponseCode == 200 || f.LastResponseCode == 0) {
			continue
		}

		fileChan <- f
	}
	close(fileChan)
	wg.Wait()

	if err := saveDatabase(); err != nil {
		log.Println(err)
	}

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

var (
	user = flag.String("user", "admin", "")
	pass = flag.String("pass", "", "")
)

func (s *server) serveSite(c *cli.Context) error {
	if err := ml.LoadOrTrainClassifier(&db, config.ClassifierDir); err != nil {
		log.Printf("Failed to load classifier. Classification tasks will not work.: %s", err)
	}

	if len(*pass) > 0 {
		secureMux := s.adminRoutes()
		http.Handle("/admin/", httpauth.SimpleBasicAuth(*user, *pass)(secureMux))
	} else {
		log.Println("No admin password set, interface disabled.")
	}

	http.HandleFunc("/upload", s.handleFileUpload)
	http.Handle("/", http.FileServer(http.Dir("static")))

	// Launch 4 source workers
	for i := 0; i < workers.Count; i++ {
		go unprocessedSourceWorker()
	}

	bindAddr := net.JoinHostPort("0.0.0.0", strconv.Itoa(c.Int("port")))
	log.Printf("Listening on %s...", bindAddr)
	return http.ListenAndServe(bindAddr, nil)
}

var (
	dbType   = flag.String("dbtype", "postgres", "the database connection type")
	dbParams = flag.String("db", "host=localhost user=examdb dbname=examdb sslmode=disable password=examdb", "the database connection string")
)

type server struct {
	db *db.DB
}

func main() {
	log.SetOutput(os.Stderr)
	log.SetFlags(log.Flags() | log.Lshortfile)

	flag.Parse()

	var err error
	db, err = db.Open(*dbType, *dbParams)
	if err != nil {
		return log.Fatal("failed to connect to db: %+v", err)
	}

	if err := loadDatabase(); err != nil {
		log.Printf("tried to load database: %s", err)
	}

	generator, err = generators.MakeGenerator(&db, config.ExamsDir)
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		if err := exambot.Run(db); err != nil {
			log.Fatal(err)
		}
	}()

	s := server{
		db: db,
	}

	if err := s.serveSite(nil); err != nil {
		log.Fatal(err)
	}
}
