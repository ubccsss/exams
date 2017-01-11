package generators

import (
	"html/template"

	"github.com/pkg/errors"
	"github.com/ubccsss/exams/examdb"
)

// Generator contains all generators.
type Generator struct {
	db                   *examdb.Database
	templates            *template.Template
	coursePotentialFiles map[string][]*examdb.File
	layout               string
	examsDir             string
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
		examsDir:  examsDir,
	}
	layout, err := g.fetchTemplate()
	if err != nil {
		return nil, err
	}
	g.layout = layout

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
