package generators

import (
	"bytes"
	"io/ioutil"
	"os"
	"path"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/pkg/errors"
	"github.com/russross/blackfriday"
)

// Database generates an index of all courses.
func (g Generator) Database() error {
	dir := g.examsDir
	if err := os.MkdirAll(dir, 0755); err != nil {
		return errors.Wrapf(err, "mkdirall %q", dir)
	}

	type course struct {
		Name               string
		Desc               string
		FileCount          int
		PotentialFileCount int
	}
	type courses map[string]course
	type level map[string]courses

	l := level{}
	for _, c := range g.db.Courses {
		cl := strings.ToUpper(c.Code[0:3] + "00")
		cs, ok := l[cl]
		if !ok {
			cs = courses{}
			l[cl] = cs
		}
		cs[c.Code] = course{
			Name:               strings.ToUpper(c.Code),
			Desc:               c.Desc,
			FileCount:          c.FileCount(),
			PotentialFileCount: len(g.coursePotentialFiles[c.Code]),
		}
	}

	var buf bytes.Buffer
	if err := g.templates.ExecuteTemplate(&buf, "index.md", l); err != nil {
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
	doc.Find("table").AddClass("table")
	htmlStr, err := doc.Html()
	if err != nil {
		return err
	}
	styled := g.renderTemplate("Exams Database", htmlStr)
	if err := ioutil.WriteFile(path.Join(dir, "index.html"), []byte(styled), 0755); err != nil {
		return err
	}
	return nil
}
