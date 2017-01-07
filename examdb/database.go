package examdb

import (
	"fmt"
	"io"
	"strings"
	"sync"
)

// Database stores all of the courses and files.
type Database struct {
	Courses        map[string]*Course `json:",omitempty"`
	PotentialFiles []*File            `json:",omitempty"`

	UnprocessedSources   []*File    `json:",omitempty"`
	UnprocessedSourcesMu sync.Mutex `json:"-"`

	SourceHashes map[string]string `json:",omitempty"`
}

// CoursesNoFiles returns the courses with no files.
func (db Database) CoursesNoFiles() []string {
	var classes []string
	for id, c := range db.Courses {
		if c.FileCount() == 0 {
			classes = append(classes, id)
		}
	}
	return classes
}

// FindFile returns the file with the matching hash and the potentialFile index
// if it's a potential file.
func (db Database) FindFile(hash string) (*File, int) {
	for i, f := range db.PotentialFiles {
		if f.Hash == hash {
			return f, i
		}
	}
	for _, course := range db.Courses {
		for _, year := range course.Years {
			for _, f := range year.Files {
				if f.Hash == hash {
					return f, -1
				}
			}
		}
	}
	return nil, -1
}

func validFileName(name string) bool {
	for _, label := range ExamLabels {
		if name == label {
			return true
		}
	}
	return false
}

// NeedFix returns all files that are potentially missing required information.
func (db Database) NeedFix() []*File {
	var files []*File
	for _, course := range db.Courses {
		for _, year := range course.Years {
			for _, f := range year.Files {
				if !validFileName(f.Name) {
					files = append(files, f)
					continue
				}

				if f.Term == "" {
					files = append(files, f)
				}
			}
		}
	}
	return files
}

// AddCourse adds a course the DB if it doesn't exist already.
func (db *Database) AddCourse(w io.Writer, code string) {
	code = strings.ToLower(strings.TrimSpace(code))
	if _, ok := db.Courses[code]; ok {
		return
	}
	if db.Courses == nil {
		db.Courses = map[string]*Course{}
	}
	db.Courses[code] = &Course{Code: code}
	fmt.Fprintf(w, "Added: %s\n", code)
}

func (db *Database) AddFile(course string, year int, term, name, path, source string) error {
	if _, ok := db.Courses[course]; !ok {
		db.Courses[course] = &Course{Code: course, Years: map[int]*CourseYear{}}
	}
	if _, ok := db.Courses[course].Years[year]; !ok {
		if db.Courses[course].Years == nil {
			db.Courses[course].Years = map[int]*CourseYear{}
		}
		db.Courses[course].Years[year] = &CourseYear{}
	}
	courseYear := db.Courses[course].Years[year]

	f := &File{Name: name, Path: path, Source: source, Term: term}
	if err := f.ComputeHash(); err != nil {
		return err
	}

	for _, file := range courseYear.Files {
		if file.Hash == f.Hash {
			file.Name = name
			file.Source = source
			file.Term = term
			return nil
		}
	}

	courseYear.Files = append(courseYear.Files, f)
	return nil
}

func (db *Database) ProcessedCount() int {
	count := 0
	for _, course := range db.Courses {
		count += course.FileCount()
	}
	return count
}

func (db *Database) Hashes() map[string]struct{} {
	m := map[string]struct{}{}

	for _, course := range db.Courses {
		for _, year := range course.Years {
			for _, f := range year.Files {
				if len(f.Hash) > 0 {
					m[f.Hash] = struct{}{}
				}
			}
		}
	}

	for _, f := range db.PotentialFiles {
		if len(f.Hash) > 0 {
			m[f.Hash] = struct{}{}
		}
	}

	return m
}

func (db *Database) AddPotentialFiles(w io.Writer, files []*File) {
	m := db.Hashes()
	var unhashed []*File
	for _, f := range files {
		if len(f.Hash) == 0 {
			fmt.Fprintf(w, "missing Hash for %+v, skipping...\n", f)
			unhashed = append(unhashed, f)
			continue
		}

		if _, ok := m[f.Hash]; ok {
			fmt.Fprintf(w, "duplicate %+v, skipping...\n", f)
			continue
		}

		m[f.Hash] = struct{}{}
		db.PotentialFiles = append(db.PotentialFiles, f)
	}
	db.UnprocessedSourcesMu.Lock()
	defer db.UnprocessedSourcesMu.Unlock()
	db.UnprocessedSources = append(db.UnprocessedSources, unhashed...)
}
