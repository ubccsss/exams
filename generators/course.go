package generators

import (
	"bytes"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
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
func (g *Generator) Course(c *examdb.Course) error {
	// Don't generate courses for unclassified files.
	if len(c.Code) == 0 {
		return nil
	}

	dir := path.Join(g.examsDir, c.Code)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	files := g.courseFiles[c.Code]
	potentialFiles := g.coursePotentialFiles[c.Code]
	sort.Sort(examdb.FileByYearTermName(potentialFiles))

	fileNames := map[string]string{}
	for _, f := range potentialFiles {
		fileNames[f.Hash] = filepath.Base(f.Source)
	}

	var pendingML []*examdb.File
	var completedML []*examdb.File
	for _, f := range potentialFiles {
		if f.Inferred != nil {
			completedML = append(completedML, f)
		} else {
			pendingML = append(pendingML, f)
		}
	}

	data := struct {
		*examdb.Course
		Years          map[int][]*examdb.File
		FileNames      map[string]string
		YearSections   []int
		PotentialFiles []*examdb.File
		CompletedML    []*examdb.File
		PendingML      []*examdb.File
	}{
		Course:         c,
		PotentialFiles: potentialFiles,
		Years:          fileTree(files),
		YearSections:   examdb.AllYears(files),
		FileNames:      fileNames,
		CompletedML:    completedML,
		PendingML:      pendingML,
	}

	var buf bytes.Buffer
	if err := Templates.ExecuteTemplate(&buf, "course.md", data); err != nil {
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

func (g *Generator) indexCourseFiles() {
	g.db.Mu.RLock()
	defer g.db.Mu.RUnlock()

	classified := map[string][]*examdb.File{}
	potential := map[string][]*examdb.File{}
	for _, f := range g.db.Files {
		if f.HandClassified {
			classified[f.Course] = append(classified[f.Course], f)
			continue
		}

		if f.NotAnExam || (f.Inferred != nil && f.Inferred.NotAnExam) {
			continue
		}

		var predicted string
		if f.Inferred != nil {
			predicted = f.Inferred.Course
		} else {
			predicted = ml.ExtractCourse(g.db, f)
		}
		potential[predicted] = append(potential[predicted], f)
	}
	g.courseFiles = classified
	g.coursePotentialFiles = potential
}
