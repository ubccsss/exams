package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/alecthomas/units"
	"github.com/pkg/errors"
	"github.com/russross/blackfriday"
)

var templates = template.Must(template.ParseGlob("template/*"))

var yearRegex = regexp.MustCompile("(20|19)\\d{2}")

// Course ...
type Course struct {
	Code  string
	Years map[int]*CourseYear
}

// Generate generates a course.
func (c Course) Generate() error {
	dir := path.Join(examsDir, c.Code)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, "course.md", c); err != nil {
		return err
	}
	if err := ioutil.WriteFile(path.Join(dir, "index.md"), buf.Bytes(), 0755); err != nil {
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
	doc.Find("h1").AddClass("page-header")
	htmlStr, err := doc.Html()
	if err != nil {
		return err
	}
	styled := renderTemplateExam(c.Code, htmlStr)
	if err := ioutil.WriteFile(path.Join(dir, "index.html"), []byte(styled), 0755); err != nil {
		return err
	}
	return nil
}

// FileCount returns the number of files for that course.
func (c Course) FileCount() int {
	count := 0
	for _, year := range c.Years {
		count += len(year.Files)
	}
	return count
}

// Generate generates an index of all courses.
func (db Database) Generate() error {
	dir := examsDir
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	type course struct {
		Name      string
		FileCount int
	}
	type courses map[string]course
	type level map[string]courses

	l := level{}
	for _, c := range db.Courses {
		cl := strings.ToUpper(c.Code[0:3] + "00")
		cs, ok := l[cl]
		if !ok {
			cs = courses{}
			l[cl] = cs
		}
		cs[c.Code] = course{Name: strings.ToUpper(c.Code), FileCount: c.FileCount()}
	}

	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, "index.md", l); err != nil {
		return err
	}
	if err := ioutil.WriteFile(path.Join(dir, "index.md"), buf.Bytes(), 0755); err != nil {
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
	doc.Find("h1").AddClass("page-header")
	htmlStr, err := doc.Html()
	if err != nil {
		return err
	}
	styled := renderTemplate("Exams Database", htmlStr)
	if err := ioutil.WriteFile(path.Join(dir, "index.html"), []byte(styled), 0755); err != nil {
		return err
	}
	return nil
}

// CourseYear ...
type CourseYear struct {
	Files []*File
}

// File ...
type File struct {
	Name      string
	Path      string
	Source    string
	Hash      string
	Score     float64
	Term      string
	NotAnExam bool
}

func (f *File) reader() (io.ReadCloser, error) {
	var source io.ReadCloser
	if len(f.Path) > 0 {
		var err error
		source, err = os.Open(f.Path)
		if err != nil {
			return nil, err
		}
	} else if len(f.Source) > 0 {
		req, err := http.Get(f.Source)
		if err != nil {
			return nil, err
		}
		source = req.Body
	} else {
		return nil, errors.Errorf("No source or path for %+v", f)
	}
	return source, nil
}

var maxFileSize = int64(10 * units.MB)

func (f *File) hash() error {
	hasher := sha1.New()
	source, err := f.reader()
	if err != nil {
		return err
	}
	defer source.Close()
	if _, err := io.Copy(hasher, io.LimitReader(source, maxFileSize)); err != nil {
		return err
	}
	f.Hash = hex.EncodeToString(hasher.Sum(nil))
	return nil
}

var scoreRegexes = map[int][]string{
	1:  []string{"final", "exam", "midterm", "sample", "mt", "(cs|cpsc)\\d{3}", "(20|19)\\d{2}"},
	-1: []string{"report", "presentation", "thesis", "slide"},
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

var regexCache = map[string]*regexp.Regexp{}

func regexpMatch(pattern, path string) bool {
	r, ok := regexCache[pattern]
	if !ok {
		r = regexp.MustCompile(pattern)
		regexCache[pattern] = r
	}

	return r.FindIndex([]byte(path)) != nil
}

func (f *File) score() {
	path := strings.ToLower(f.Source)
	var score int
	for _, r := range db.CoursesNoFiles() {
		if regexpMatch(r, path) {
			score++
		}
	}
	for s, rs := range scoreRegexes {
		for _, r := range rs {
			if regexpMatch(r, path) {
				score += s
			}
		}
	}
	f.Score = float64(score)
}

// FileSlice attaches the methods of sort.Interface to []*File, sorting in increasing order.
type FileSlice []*File

func (p FileSlice) Len() int           { return len(p) }
func (p FileSlice) Less(i, j int) bool { return p[i].Score >= p[j].Score }
func (p FileSlice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

// Database ...
type Database struct {
	Courses            map[string]*Course
	PotentialFiles     []*File
	UnprocessedSources []*File
	SourceHashes       map[string]string
}

var unprocessedSourcesMu sync.Mutex

func unprocessedSourceWorker() {
	for {
		if len(db.UnprocessedSources) == 0 {
			time.Sleep(1 * time.Second)
			continue
		}

		var f *File
		unprocessedSourcesMu.Lock()
		if len(db.UnprocessedSources) > 0 {
			f = db.UnprocessedSources[len(db.UnprocessedSources)-1]
			db.UnprocessedSources = db.UnprocessedSources[:len(db.UnprocessedSources)-1]
		}
		unprocessedSourcesMu.Unlock()

		if f == nil {
			continue
		}

		if err := f.hash(); err != nil {
			log.Printf("error processing source: %+v: %s", f, err)
		}

		db.addPotentialFiles(os.Stderr, []*File{f})
		log.Printf("%d remaining unprocessed sources", len(db.UnprocessedSources))
	}
}

func (db *Database) addFile(course string, year int, term, name, path, source string) error {
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

	for _, file := range courseYear.Files {
		if file.Path == path {
			file.Name = name
			file.Source = source
			return nil
		}
	}

	f := &File{Name: name, Path: path, Source: source}
	if err := f.hash(); err != nil {
		return err
	}
	courseYear.Files = append(courseYear.Files, f)
	return nil
}

func (db *Database) processedCount() int {
	count := 0
	for _, course := range db.Courses {
		count += course.FileCount()
	}
	return count
}

func (db *Database) hashes() map[string]struct{} {
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

func (db *Database) addPotentialFiles(w io.Writer, files []*File) {
	m := db.hashes()
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
	unprocessedSourcesMu.Lock()
	defer unprocessedSourcesMu.Unlock()
	db.UnprocessedSources = append(db.UnprocessedSources, unhashed...)
}

const staticDir = "static"
const examsDir = "static/exams"

const dbFile = "exams.json"

var db Database

func loadDatabase() error {
	raw, err := ioutil.ReadFile(dbFile)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, &db); err != nil {
		return err
	}
	if err := verifyConsistency(); err != nil {
		return err
	}
	return nil
}

func saveDatabase() error {
	raw, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		return err
	}
	if err := ioutil.WriteFile(dbFile, raw, 0755); err != nil {
		return err
	}
	return nil
}

func createDirs() error {
	db.Generate()

	for _, course := range db.Courses {
		if err := course.Generate(); err != nil {
			return err
		}
	}
	return nil
}

func verifyConsistency() error {
	log.Println("Verifying consistency of data and doing house keeping...")
	for _, course := range db.Courses {
		for _, year := range course.Years {
			for _, f := range year.Files {
				if len(f.Hash) == 0 {
					if err := f.hash(); err != nil {
						return err
					}
				}
			}
		}
	}

	for _, f := range db.PotentialFiles {
		f.score()
		f.Path = strings.TrimSpace(f.Path)
		f.Source = strings.TrimSpace(f.Source)
	}
	sort.Sort(FileSlice(db.PotentialFiles))

	return nil
}

func generate() error {
	return createDirs()
}

func saveAndGenerate() error {
	if err := generate(); err != nil {
		return err
	}
	if err := saveDatabase(); err != nil {
		return err
	}
	return nil
}

var layout string

func main() {
	if err := loadDatabase(); err != nil {
		log.Printf("tried to load database: %s", err)
	}

	wrapperTemplate, err := fetchTemplate()
	if err != nil {
		log.Fatal(err)
	}
	layout = wrapperTemplate

	http.Handle("/static/", http.FileServer(http.Dir(".")))

	http.HandleFunc("/admin/generate", handleGenerate)
	http.HandleFunc("/admin/potential", handlePotentialFileIndex)
	http.HandleFunc("/admin/file/", handleFile)

	http.HandleFunc("/admin/ingress/deptcourses", ingressDeptCourses)
	http.HandleFunc("/admin/ingress/deptfiles", ingressDeptFiles)
	http.HandleFunc("/admin/ingress/ubccsss", ingressUBCCSSS)
	http.HandleFunc("/admin/ingress/archive.org", ingressArchiveOrgFiles)

	// Launch 4 source workers
	for i := 0; i < 4; i++ {
		go unprocessedSourceWorker()
	}

	log.Println("Listening...")
	log.Fatal(http.ListenAndServe("0.0.0.0:8080", nil))
}

func fetchFileAndSave(course string, year int, term, name, href string) error {
	resp, err := http.Get(href)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	base := path.Base(href)
	dir := fmt.Sprintf("%s/%s/%d", examsDir, course, year)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	raw, _ := ioutil.ReadAll(resp.Body)
	file := path.Join(dir, base)
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		return errors.New("file already exists!")
	}
	if err := ioutil.WriteFile(file, raw, 0755); err != nil {
		return err
	}
	db.addFile(course, year, term, name, file, href)
	return nil
}
