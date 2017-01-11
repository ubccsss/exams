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

// Course generates a course.
func (g Generator) Course(c examdb.Course) error {
	dir := path.Join(g.examsDir, c.Code)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data := struct {
		examdb.Course
		YearSections   []int
		PotentialFiles []*examdb.File
	}{
		Course:         c,
		PotentialFiles: g.coursePotentialFiles[c.Code],
	}

	for y := range c.Years {
		data.YearSections = append(data.YearSections, y)
	}

	sort.Sort(sort.Reverse(sort.IntSlice(data.YearSections)))

	var buf bytes.Buffer
	if err := g.templates.ExecuteTemplate(&buf, "course.md", data); err != nil {
		return err
	}
	/*
		if err := ioutil.WriteFile(path.Join(dir, "index.md"), buf.Bytes(), 0755); err != nil {
			return err
		}
	*/
	html := blackfriday.MarkdownCommon(buf.Bytes())
	buf.Reset()
	if _, err := buf.Write(html); err != nil {
		return err
	}
	doc, err := goquery.NewDocumentFromReader(&buf)
	if err != nil {
		return err
	}
	doc.Find("h1").AddClass("page-header")
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
	for _, f := range g.db.PotentialFiles {
		if f.NotAnExam {
			continue
		}

		predicted := ml.ExtractCourse(g.db, f)
		m[predicted] = append(m[predicted], f)
	}
	g.coursePotentialFiles = m
}
