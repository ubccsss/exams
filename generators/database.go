package generators

import (
	"bytes"

	"github.com/PuerkitoBio/goquery"
	"github.com/russross/blackfriday"
	"github.com/ubccsss/exams/db"
)

// Database generates an index of all courses.
func (g *Generator) Database() (string, error) {
	type course struct {
		Name               string
		Desc               string
		FileCount          int
		PotentialFileCount int
	}
	type courses map[string]course
	type level map[string]courses

	l := level{}
	cs, err := g.dbNew.Courses(db.Course{})
	for _, c := range cs {
		cl := c.YearLevel()
		cs, ok := l[cl]
		if !ok {
			cs = courses{}
			l[cl] = cs
		}
		cs[c.CanonicalURL()] = course{
			Name:               c.Title(),
			Desc:               c.Desc,
			FileCount:          -1, // TODO
			PotentialFileCount: -1, // TODO
		}
	}

	var buf bytes.Buffer
	if err := Templates.ExecuteTemplate(&buf, "index.md", l); err != nil {
		return "", err
	}
	html := blackfriday.MarkdownCommon(buf.Bytes())
	buf.Reset()
	if _, err := buf.Write(html); err != nil {
		return "", err
	}
	doc, err := goquery.NewDocumentFromReader(&buf)
	if err != nil {
		return "", err
	}
	addStyleClasses(doc)
	htmlStr, err := doc.Html()
	if err != nil {
		return "", err
	}
	return g.renderTemplate("Exams Database", htmlStr), nil
}
