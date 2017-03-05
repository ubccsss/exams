package generators

import (
	"html/template"
	"log"
	"sync"

	"github.com/PuerkitoBio/goquery"
	"github.com/pkg/errors"
	"github.com/ubccsss/exams/examdb"
)

// Generator contains all generators.
type Generator struct {
	db                   *examdb.Database
	templates            *template.Template
	coursePotentialFiles map[string][]*examdb.File
	layout               string
	layoutOnce           sync.Once
	examsDir             string
}

// MakeGenerator creates a new generator and loads all data required.
func MakeGenerator(db *examdb.Database, examsDir string) (*Generator, error) {
	g := &Generator{
		db:        db,
		templates: Templates,
		examsDir:  examsDir,
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
	g.indexCoursePotentialFiles()

	if err := g.Database(); err != nil {
		return errors.Wrap(err, "database")
	}

	for _, course := range g.db.Courses {
		if err := g.Course(*course); err != nil {
			return errors.Wrapf(err, "course %+v", course)
		}
	}
	return nil
}

func addStyleClasses(sel *goquery.Document) {
	sel.Find("h1").AddClass("page-header")
	sel.Find("table").AddClass("table")
}
