package generators

import (
	"html/template"

	"github.com/d4l3k/exams/examdb"
)

// Generator contains all generators.
type Generator struct {
	db        *examdb.Database
	templates *template.Template
	layout    string
	examsDir  string
}

// MakeGenerator creates a new generator and loads all data required.
func MakeGenerator(db *examdb.Database, templateGlob, examsDir string) (*Generator, error) {
	templates, err := template.ParseGlob(templateGlob)
	if err != nil {
		return nil, err
	}
	g := &Generator{
		db:        db,
		templates: templates,
	}
	layout, err := g.fetchTemplate()
	if err != nil {
		return nil, err
	}
	g.layout = layout

	return g, nil
}

// All generates all files that there are generates for.
func (g Generator) All() error {
	if err := g.Database(); err != nil {
		return err
	}

	for _, course := range g.db.Courses {
		if err := g.Course(*course); err != nil {
			return err
		}
	}
	return nil
}
