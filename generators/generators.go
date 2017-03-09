package generators

import (
	"log"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/pkg/errors"
	"github.com/ubccsss/exams/examdb"
	"github.com/ubccsss/exams/workers"
)

// Generator contains all generators.
type Generator struct {
	db                   *examdb.Database
	courseFiles          map[string][]*examdb.File
	coursePotentialFiles map[string][]*examdb.File
	layout               string
	layoutOnce           sync.Once
	examsDir             string
}

// MakeGenerator creates a new generator and loads all data required.
func MakeGenerator(db *examdb.Database, examsDir string) (*Generator, error) {
	g := &Generator{
		db:       db,
		examsDir: examsDir,
	}

	go func() {
		if err := g.fetchLayout(); err != nil {
			log.Println(err)
		}
	}()

	return g, nil
}

// All generates all files that there are generates for.
func (g *Generator) All() error {
	start := time.Now()
	g.indexCourseFiles()

	var wg sync.WaitGroup

	wg.Add(1)
	var dbErr error
	go func() {
		defer wg.Done()

		start := time.Now()
		if err := g.Database(); err != nil {
			dbErr = errors.Wrap(err, "database")
			return
		}
		log.Printf("Generated index in %s.", time.Since(start))
	}()

	startCourses := time.Now()

	courseChan := make(chan *examdb.Course, workers.Count)
	errorChan := make(chan error, workers.Count)
	for i := 0; i < workers.Count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for course := range courseChan {
				if err := g.Course(course); err != nil {
					errorChan <- errors.Wrapf(err, "course %+v", course)
					return
				}
			}
		}()
	}

	for _, course := range g.db.Courses {
		courseChan <- course
	}
	close(courseChan)
	wg.Wait()
	close(errorChan)
	if dbErr != nil {
		return dbErr
	}
	for err := range errorChan {
		return err
	}

	log.Printf("Generated in %s. Course pages in %s.", time.Since(start), time.Since(startCourses))

	return nil
}

func addStyleClasses(sel *goquery.Document) {
	sel.Find("h1").AddClass("page-header")
	sel.Find("table").AddClass("table")
}
