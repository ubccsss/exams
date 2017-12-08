package generators

import (
	"log"
	"sync"

	"github.com/PuerkitoBio/goquery"
	"github.com/ubccsss/exams/db"
	"github.com/ubccsss/exams/examdb"
)

// Generator contains all generators.
type Generator struct {
	db                   *examdb.Database
	dbNew                *db.DB
	courseFiles          map[string][]*examdb.File
	coursePotentialFiles map[string][]*examdb.File
	layout               string
	layoutOnce           sync.Once
}

// MakeGenerator creates a new generator and loads all data required.
func MakeGenerator(db *db.DB) (*Generator, error) {
	g := &Generator{
		dbNew: db,
	}

	go func() {
		if err := g.fetchLayout(); err != nil {
			log.Println(err)
		}
	}()

	return g, nil
}

func addStyleClasses(sel *goquery.Document) {
	sel.Find("h1").AddClass("page-header")
	sel.Find("table").AddClass("table")
}
