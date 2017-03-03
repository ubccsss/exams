package generators

import (
	"bytes"
	"io/ioutil"
	"log"
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
	fileCounts := g.db.CourseFileCount()
	log.Printf("fileCounts %+v", fileCounts)
	for _, c := range g.db.Courses {
		// Don't render unclassified files.
		if len(c.Code) == 0 {
			continue
		}
		cl := c.YearLevel()
		cs, ok := l[cl]
		if !ok {
			cs = courses{}
			l[cl] = cs
		}
		count := fileCounts[c.Code]
		cs[c.Code] = course{
			Name:               strings.ToUpper(c.Code),
			Desc:               c.Desc,
			FileCount:          count.HandClassified,
			PotentialFileCount: count.Potential,
		}
	}

	var buf bytes.Buffer
	if err := g.templates.ExecuteTemplate(&buf, "index.md", l); err != nil {
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
	styled := g.renderTemplate("Exams Database", htmlStr)
	if err := ioutil.WriteFile(path.Join(dir, "index.html"), []byte(styled), 0755); err != nil {
		return err
	}
	return nil
}
