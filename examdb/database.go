package examdb

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"github.com/ubccsss/exams/config"
)

// Database stores all of the courses and files.
type Database struct {
	// Courses should not be accessed without locking Mu.
	Courses map[string]*Course `json:",omitempty"`
	// Files should not be accessed without locking Mu.
	Files []*File `json:",omitempty"`
	//PotentialFiles []*File            `json:",omitempty"`
	SourceHashes map[string]string `json:",omitempty"`
	Mu           sync.RWMutex      `json:"-"`

	UnprocessedSources   []*File      `json:",omitempty"`
	UnprocessedSourcesMu sync.RWMutex `json:"-"`
}

// MakeDatabase makes a new database.
func MakeDatabase() *Database {
	return &Database{
		Courses:      map[string]*Course{},
		SourceHashes: map[string]string{},
	}
}

// CoursesNoFiles returns the courses with no files.
func (db *Database) CoursesNoFiles() []string {
	var classes []string
	for id, count := range db.CourseFileCount() {
		if count.HandClassified == 0 {
			c := db.Courses[id]
			classes = append(classes, c.AlternateIDs()...)
		}
	}
	return classes
}

// FileCount contains file counts for a number of different categories.
type FileCount struct {
	Total, HandClassified, Potential, NotAnExam int
}

// CourseFileCount returns a map between the course ID and the number of files
// it has.
func (db *Database) CourseFileCount() map[string]FileCount {
	db.Mu.RLock()
	defer db.Mu.RUnlock()

	m := map[string]FileCount{}
	for _, f := range db.Files {
		cid := f.Course

		if len(cid) == 0 && f.Inferred != nil {
			cid = f.Inferred.Course
		}

		c := m[cid]
		if f.NotAnExam {
			c.NotAnExam++
		}
		if f.IsPotential() {
			c.Potential++
		} else {
			c.HandClassified++
		}
		c.Total++
		m[cid] = c
	}
	return m
}

// FileCount returns the file stats for the database.
func (db *Database) FileCount() FileCount {
	var c FileCount
	for _, count := range db.CourseFileCount() {
		c.HandClassified += count.HandClassified
		c.Potential += count.Potential
		c.NotAnExam += count.NotAnExam
		c.Total += count.Total
	}
	return c
}

// FindFile returns the file with the matching hash and the potentialFile index
// if it's a potential file.
func (db *Database) FindFile(hash string) *File {
	db.Mu.RLock()
	defer db.Mu.RUnlock()

	return db.findFileLocked(hash)
}

func (db *Database) findFileLocked(hash string) *File {
	for _, f := range db.Files {
		if f.Hash == hash {
			return f
		}
	}
	return nil
}

// FindFileByPath returns the file with the matching path.
func (db *Database) FindFileByPath(fp string) *File {
	db.Mu.RLock()
	defer db.Mu.RUnlock()

	if len(fp) == 0 {
		return nil
	}

	for _, f := range db.Files {
		// Allow matching "/0/foo" and "0/foo" due to a bug causing the first to be
		// generated.
		if path.Join("/", f.Path) == path.Join("/", fp) {
			return f
		}
	}
	return nil
}

// FindCourseFiles returns all files with the specified course.
func (db *Database) FindCourseFiles(c *Course) []*File {
	db.Mu.RLock()
	defer db.Mu.RUnlock()

	var files []*File
	for _, f := range db.Files {
		if f.Course != c.Code {
			continue
		}

		files = append(files, f)
	}
	return files
}

// ProcessedFiles returns all files that have been "processed" or are not
// potential.
func (db *Database) ProcessedFiles() []*File {
	db.Mu.RLock()
	defer db.Mu.RUnlock()

	var files []*File
	for _, f := range db.Files {
		if f.IsPotential() {
			continue
		}

		files = append(files, f)
	}
	return files
}

// DisplayCourses returns all course codes that should be displayed in
// alphabetical order.
func (db *Database) DisplayCourses() []string {
	db.Mu.RLock()
	defer db.Mu.RUnlock()

	var courses []string
	for code, c := range db.Courses {
		if config.DisplayDepartment[c.Department()] {
			courses = append(courses, code)
		}
	}
	sort.Strings(courses)
	return courses
}

// UnprocessedFiles returns all files that haven't been processed.
func (db *Database) UnprocessedFiles() []*File {
	db.Mu.RLock()
	defer db.Mu.RUnlock()

	var files []*File
	for _, f := range db.Files {
		if !f.IsPotential() {
			continue
		}

		files = append(files, f)
	}
	return files
}

// NotAnExamFiles returns all files that aren't exams.
func (db *Database) NotAnExamFiles() []*File {
	db.Mu.RLock()
	defer db.Mu.RUnlock()

	var files []*File
	for _, f := range db.Files {
		if !f.NotAnExam {
			continue
		}

		files = append(files, f)
	}
	return files
}

// AllYears returns all years in the list of files.
func AllYears(files []*File) []int {
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

func validFileName(name string) bool {
	for _, label := range ExamLabels {
		if name == label {
			return true
		}
	}
	return false
}

// NeedFixReasons contains a file and the reasons it needs to be fixed.
type NeedFixReasons struct {
	File    *File
	Reasons []string
}

// NeedFix returns all files that are potentially missing required information.
func (db *Database) NeedFix() []NeedFixReasons {
	db.Mu.RLock()
	defer db.Mu.RUnlock()

	var files []NeedFixReasons
	var reasons NeedFixReasons
	for _, f := range db.Files {
		if !f.HandClassified {
			continue
		}

		reasons = NeedFixReasons{
			File:    f,
			Reasons: nil,
		}

		if !validFileName(f.Name) {
			reasons.Reasons = append(reasons.Reasons, "invalid file name")
		}
		if f.Term == "" {
			reasons.Reasons = append(reasons.Reasons, "missing term")
		}
		if f.Year == 0 {
			reasons.Reasons = append(reasons.Reasons, "missing year")
		}

		if len(reasons.Reasons) > 0 {
			files = append(files, reasons)
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
	if _, ok := db.Courses[course]; !ok {
		db.Courses[course] = &Course{Code: course}
	}

	if err := f.ComputeHash(); err != nil {
		return err
	}

	found := db.findFileLocked(f.Hash)
	if found == nil {
		db.Files = append(db.Files, f)
	} else {
		*found = *f
	}

	return nil
}

// ProcessedCount returns the number of files that have been processed.
func (db *Database) ProcessedCount() int {
	db.Mu.RLock()
	defer db.Mu.RUnlock()

	count := 0
	for _, f := range db.Files {
		if !f.IsPotential() {
			count++
		}
	}
	return count
}

// Hashes returns a map with all hashes in the DB.
func (db *Database) Hashes() map[string]struct{} {
	db.Mu.RLock()
	defer db.Mu.RUnlock()

	m := map[string]struct{}{}

	for _, f := range db.Files {
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
		db.Files = append(db.Files, f)
	}
	db.UnprocessedSourcesMu.Lock()
	defer db.UnprocessedSourcesMu.Unlock()
	db.UnprocessedSources = append(db.UnprocessedSources, unhashed...)
}

// FetchFileAndSave fetches the file and saves it to a directory.
func (db *Database) FetchFileAndSave(file *File) error {
	log.Printf("Fetching %q", file.Source)
	resp, err := file.Reader()
	if err != nil {
		return err
	}
	defer resp.Close()
	filename := file.Source
	if len(file.Source) == 0 {
		filename = file.Path
	}
	dir := file.IdealDir()
	if err := os.MkdirAll(path.Join(config.ExamsDir, dir), 0755); err != nil {
		return err
	}
	attempt := path.Base(filename)
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

	for i, f := range db.Files {
		if f.Hash == file.Hash {
			db.Files = append(db.Files[:i], db.Files[i+1:]...)
			return nil
		}
	}

	return errors.New("could not find file")
}
