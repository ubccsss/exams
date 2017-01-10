package examdb

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/d4l3k/exams/config"
	"github.com/pkg/errors"
)

// Database stores all of the courses and files.
type Database struct {
	Courses        map[string]*Course `json:",omitempty"`
	PotentialFiles []*File            `json:",omitempty"`
	SourceHashes   map[string]string  `json:",omitempty"`
	Mu             sync.RWMutex       `json:"-"`

	UnprocessedSources   []*File      `json:",omitempty"`
	UnprocessedSourcesMu sync.RWMutex `json:"-"`
}

// CoursesNoFiles returns the courses with no files.
func (db *Database) CoursesNoFiles() []string {
	db.Mu.RLock()
	defer db.Mu.RUnlock()

	var classes []string
	for id, c := range db.Courses {
		if c.FileCount() == 0 {
			classes = append(classes, id)
			code := id[2:]
			classes = append(classes, fmt.Sprintf("cpsc%s", code))
		}
	}
	return classes
}

// FindFile returns the file with the matching hash and the potentialFile index
// if it's a potential file.
func (db *Database) FindFile(hash string) *File {
	db.Mu.RLock()
	defer db.Mu.RUnlock()

	for _, f := range db.PotentialFiles {
		if f.Hash == hash {
			return f
		}
	}
	for _, course := range db.Courses {
		for _, year := range course.Years {
			for _, f := range year.Files {
				if f.Hash == hash {
					return f
				}
			}
		}
	}
	return nil
}

// FindFileByPath returns the file with the matching path.
func (db *Database) FindFileByPath(path string) *File {
	db.Mu.RLock()
	defer db.Mu.RUnlock()

	for _, course := range db.Courses {
		for _, year := range course.Years {
			for _, f := range year.Files {
				if f.Path == path {
					return f
				}
			}
		}
	}
	return nil
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
func (db *Database) NeedFix() []*File {
	db.Mu.RLock()
	defer db.Mu.RUnlock()

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
func (db *Database) AddCourse(w io.Writer, code, desc string) {
	db.Mu.Lock()
	defer db.Mu.Unlock()

	code = strings.ToLower(strings.TrimSpace(code))
	if c, ok := db.Courses[code]; ok {
		c.Desc = desc
		return
	}
	if db.Courses == nil {
		db.Courses = map[string]*Course{}
	}
	db.Courses[code] = &Course{Code: code, Desc: desc}
	fmt.Fprintf(w, "Added: %s\n", code)
}

// AddFile adds a file to the database.
func (db *Database) AddFile(f *File) error {
	db.Mu.Lock()
	defer db.Mu.Unlock()

	return db.addFileLocked(f)
}

func (db *Database) addFileLocked(f *File) error {
	course := f.Course
	year := f.Year
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

	if err := f.ComputeHash(); err != nil {
		return err
	}

	for _, file := range courseYear.Files {
		if file.Hash == f.Hash {
			*file = *f
			return nil
		}
	}

	courseYear.Files = append(courseYear.Files, f)
	return nil
}

// ProcessedCount returns the number of files that have been processed.
func (db *Database) ProcessedCount() int {
	db.Mu.RLock()
	defer db.Mu.RUnlock()

	count := 0
	for _, course := range db.Courses {
		count += course.FileCount()
	}
	return count
}

// Hashes returns a map with all hashes in the DB.
func (db *Database) Hashes() map[string]struct{} {
	db.Mu.RLock()
	defer db.Mu.RUnlock()

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

// AddPotentialFiles dedups and adds files to the list of potential files.
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

// FetchFileAndSave fetches the file and saves it to a directory.
func (db *Database) FetchFileAndSave(file *File) error {
	resp, err := file.Reader()
	if err != nil {
		return err
	}
	defer resp.Close()
	filename := file.Source
	if len(file.Source) == 0 {
		filename = file.Path
	}
	base := path.Base(filename)
	dir := fmt.Sprintf("%s/%d", file.Course, file.Year)
	if err := os.MkdirAll(path.Join(config.ExamsDir, dir), 0755); err != nil {
		return err
	}
	attempt := base
	for i := 0; ; i++ {
		if i > 0 {
			attempt = incrementFileName(attempt)
		}
		file.Path = path.Join(dir, attempt)
		if _, err := os.Stat(file.PathOnDisk()); !os.IsNotExist(err) {
			f2 := db.FindFileByPath(file.Path)
			if f2 == nil || f2.Hash != file.Hash {
				// One final check in case something became inconsistent.
				f3 := File{Path: file.Path}
				if err := f3.ComputeHash(); err != nil {
					return err
				}
				if f3.Hash != file.Hash {
					continue
				}
			}
		}
		break
	}
	raw, _ := ioutil.ReadAll(resp)
	if err := ioutil.WriteFile(file.PathOnDisk(), raw, 0755); err != nil {
		return err
	}
	return db.AddFile(file)
}

func incrementFileName(file string) string {
	parts := strings.Split(file, ".")
	if len(parts) == 0 {
		log.Fatal("empty file name")
	}

	ok, err := regexp.MatchString("^.*-\\d+$", parts[0])
	if err != nil {
		log.Fatal(err)
	}
	if ok {
		baseParts := strings.Split(parts[0], "-")
		n, _ := strconv.Atoi(baseParts[len(baseParts)-1])
		n++
		baseParts[len(baseParts)-1] = strconv.Itoa(n)
		parts[0] = strings.Join(baseParts, "-")
	} else {
		parts[0] += "-1"
	}
	return strings.Join(parts, ".")
}

// RemoveFile removes a file from the database.
func (db *Database) RemoveFile(file *File) error {
	db.Mu.Lock()
	defer db.Mu.Unlock()

	for i, f := range db.PotentialFiles {
		if f.Hash == file.Hash {
			db.PotentialFiles = append(db.PotentialFiles[:i], db.PotentialFiles[i+1:]...)
			return nil
		}
	}
	for _, course := range db.Courses {
		for _, year := range course.Years {
			for i, f := range year.Files {
				if f.Hash == file.Hash {
					year.Files = append(year.Files[:i], year.Files[i+1:]...)
					return nil
				}
			}
		}
	}

	return errors.New("could not find file")
}
