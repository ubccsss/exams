package generators

import (
	"bytes"
	"path/filepath"
	"sort"

	"github.com/PuerkitoBio/goquery"
	"github.com/russross/blackfriday"
	"github.com/ubccsss/exams/db"
	"github.com/ubccsss/exams/examdb"
	"github.com/ubccsss/exams/ml"
)

func fileTree(files []db.File) map[int][]db.File {
	m := map[int][]db.File{}
	for _, f := range files {
		m[f.Year] = append(m[f.Year], f)
	}
	for _, files := range m {
		sort.Sort(db.FileByTerm(files))
	}
	return m
}

// Course generates a course.
func (g *Generator) Course(c db.Course) (string, error) {
	// Don't generate courses for unclassified files.
	if len(c.Code) == 0 {
		return "", nil
	}

	allFiles, err := g.dbNew.Files(db.File{CourseCode: c.Code, CourseFaculty: c.Faculty})
	if err != nil {
		return "", err
	}
	var files []db.File
	var potentialFiles []db.File

	for _, f := range allFiles {
		if f.HandClassified {
			files = append(files, f)
		} else {
			potentialFiles = append(potentialFiles, f)
		}
	}

	fileNames := map[string]string{}
	for _, f := range potentialFiles {
		fileNames[f.Hash] = filepath.Base(f.SourceURL)
	}

	data := struct {
		Title string
		db.Course
		Years          map[int][]db.File
		FileNames      map[string]string
		YearSections   []int
		PotentialFiles []db.File
	}{
		Title:          c.Title(),
		Course:         c,
		PotentialFiles: potentialFiles,
		Years:          fileTree(files),
		YearSections:   AllYears(files),
		FileNames:      fileNames,
	}

	var buf bytes.Buffer
	if err := Templates.ExecuteTemplate(&buf, "course.md", data); err != nil {
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
	return g.renderTemplateExam(c.Title(), htmlStr), nil
}

// AllYears returns all years in the list of files.
func AllYears(files []db.File) []int {
	fileM := map[int]struct{}{}
	var years []int
	for _, f := range files {
		if _, ok := fileM[f.Year]; ok {
			continue
		}

		fileM[f.Year] = struct{}{}
		years = append(years, f.Year)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(years)))
	return years
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
