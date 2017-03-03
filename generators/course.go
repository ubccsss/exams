package generators

import (
	"bytes"
	"io/ioutil"
	"os"
	"path"
	"sort"

	"github.com/PuerkitoBio/goquery"
	"github.com/russross/blackfriday"
	"github.com/ubccsss/exams/examdb"
	"github.com/ubccsss/exams/ml"
)

func fileTree(files []*examdb.File) map[int][]*examdb.File {
	m := map[int][]*examdb.File{}
	for _, f := range files {
		m[f.Year] = append(m[f.Year], f)
	}
	for _, files := range m {
		sort.Sort(examdb.FileByTerm(files))
	}
	return m
}

// Course generates a course.
func (g Generator) Course(c examdb.Course) error {
	// Don't generate courses for unclassified files.
	if len(c.Code) == 0 {
		return nil
	}

	dir := path.Join(g.examsDir, c.Code)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	files := g.db.FindCourseFiles(c)
	data := struct {
		examdb.Course
		Years          map[int][]*examdb.File
		YearSections   []int
		PotentialFiles []*examdb.File
	}{
		Course:         c,
		PotentialFiles: g.coursePotentialFiles[c.Code],
		Years:          fileTree(files),
		YearSections:   examdb.AllYears(files),
	}

	var buf bytes.Buffer
	if err := g.templates.ExecuteTemplate(&buf, "course.md", data); err != nil {
		return err
	}
	html := blackfriday.MarkdownCommon(buf.Bytes())
	buf.Reset()
	if _, err := buf.Write(html); err != nil {
		return err
	}
	doc, err := goquery.NewDocumentFromReader(&buf)
	if err != nil {
		return err
	}
	addStyleClasses(doc)
	htmlStr, err := doc.Html()
	if err != nil {
		return err
	}
	styled := g.renderTemplateExam(c.Code, htmlStr)
	if err := ioutil.WriteFile(path.Join(dir, "index.html"), []byte(styled), 0755); err != nil {
		return err
	}
	return nil
}

func (g *Generator) indexCoursePotentialFiles() {
	m := map[string][]*examdb.File{}
	for _, f := range g.db.Files {
		if f.NotAnExam || f.Inferred != nil && f.NotAnExam || f.HandClassified {
			continue
		}

		predicted := ml.ExtractCourse(g.db, f)
		m[predicted] = append(m[predicted], f)
	}
	g.coursePotentialFiles = m
}
