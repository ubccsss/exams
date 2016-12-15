package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/russross/blackfriday"
)

var templates = template.Must(template.ParseGlob("template/*"))

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
	if err := ioutil.WriteFile(path.Join(dir, "index.html"), html, 0755); err != nil {
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
	Name  string
	Path  string
	Hash  string
	Score float64
}

func (f *File) hash() error {
	bytes, err := ioutil.ReadFile(f.Path)
	if err != nil {
		return err
	}
	hasher := sha1.New()
	hasher.Write(bytes)
	f.Hash = hex.EncodeToString(hasher.Sum(nil))
	return nil
}

var scoreRegexes = []string{
	"final", "exam", "midterm", "sample", "mt", "cs\\d{3}", "20\\d{2}",
}

func (f *File) score() {
	path := strings.ToLower(f.Path)
	var score float64
	for _, r := range scoreRegexes {
		matched, err := regexp.MatchString(r, path)
		if err != nil {
			log.Fatal(err)
		}
		if matched {
			score++
		}
	}
	f.Score = score
}

// FileSlice attaches the methods of sort.Interface to []*File, sorting in increasing order.
type FileSlice []*File

func (p FileSlice) Len() int           { return len(p) }
func (p FileSlice) Less(i, j int) bool { return p[i].Score >= p[j].Score }
func (p FileSlice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

// Database ...
type Database struct {
	Courses        map[string]*Course
	PotentialFiles []*File
}

func (db *Database) addFile(course string, year int, name, path string) error {
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
			return nil
		}
	}

	courseYear.Files = append(courseYear.Files, &File{Name: name, Path: path})
	return nil
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

func (db *Database) addPotentialFiles(w http.ResponseWriter, files []*File) {
	m := db.hashes()
	for _, f := range files {
		if len(f.Hash) == 0 {
			fmt.Fprintf(w, "missing Hash for %+v, skipping...\n", f)
			continue
		}

		if _, ok := m[f.Hash]; ok {
			fmt.Fprintf(w, "duplicate %+v, skipping...\n", f)
			continue
		}

		m[f.Hash] = struct{}{}
		db.PotentialFiles = append(db.PotentialFiles, f)
	}
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
	for _, course := range db.Courses {
		if err := course.Generate(); err != nil {
			return err
		}
	}
	return nil
}

func verifyConsistency() error {
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

func main() {
	if err := loadDatabase(); err != nil {
		log.Printf("tried to load database: %s", err)
	}

	http.Handle("/static/", http.FileServer(http.Dir(".")))
	http.HandleFunc("/admin/loadcourses", func(w http.ResponseWriter, r *http.Request) {
		resp, err := exec.Command("ssh", "q7w9a@remote.ugrad.cs.ubc.ca", "-C", "ls /home/c").Output()
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		parts := bytes.Split(resp, []byte("\n"))
		courseRegex := regexp.MustCompile("^cs\\d{3}$")
		for _, part := range parts {
			if !courseRegex.Match(part) {
				continue
			}
			course := string(part)
			if _, ok := db.Courses[course]; ok {
				continue
			}
			if db.Courses == nil {
				db.Courses = map[string]*Course{}
			}
			db.Courses[course] = &Course{Code: course}
			fmt.Fprintf(w, "Added: %s\n", course)
		}

		fmt.Fprintf(w, "Done.")

		if err := saveAndGenerate(); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	})

	http.HandleFunc("/admin/generate", func(w http.ResponseWriter, r *http.Request) {
		if err := saveAndGenerate(); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	})

	http.HandleFunc("/admin/potential", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<ul>")
		for _, file := range db.PotentialFiles {
			fmt.Fprintf(w, `<li><a href="/admin/file/%s">%s</a></li>`, file.Hash, file.Path)
		}
		fmt.Fprint(w, "</ul>")
	})

	http.HandleFunc("/admin/file/", func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(r.URL.Path, "/")
		hash := parts[len(parts)-1]
		var file *File
		var filei int
		for i, f := range db.PotentialFiles {
			if f.Hash == hash {
				file = f
				filei = i
				break
			}
		}
		if file == nil {
			http.Error(w, "not found", 404)
			return
		}

		if r.Method == "POST" {
			if err := r.ParseForm(); err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			course := r.FormValue("course")
			if len(course) == 0 {
				http.Error(w, "must specify course", 400)
				return
			}
			name := r.FormValue("name")
			if len(name) == 0 {
				http.Error(w, "must specify name", 400)
				return
			}
			year, err := strconv.Atoi(r.FormValue("year"))
			if err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			fetchFileAndSave(w, course, year, name, file.Path)
			db.PotentialFiles = append(db.PotentialFiles[:filei], db.PotentialFiles[filei+1:]...)
			if err := saveAndGenerate(); err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			http.Redirect(w, r, "/admin/potential", 302)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		meta := struct {
			File    *File
			Courses map[string]*Course
		}{file, db.Courses}
		if err := templates.ExecuteTemplate(w, "file.html", meta); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	})

	http.HandleFunc("/admin/ingressdept", func(w http.ResponseWriter, r *http.Request) {
		req, err := http.Get("https://www.ugrad.cs.ubc.ca/~q7w9a/exams.cgi/exams.cgi/")
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		var files []*File
		if err := json.NewDecoder(req.Body).Decode(&files); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		db.addPotentialFiles(w, files)
		if err := saveAndGenerate(); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		fmt.Fprintf(w, "Done.")
	})

	http.HandleFunc("/admin/ubccsss", func(w http.ResponseWriter, r *http.Request) {
		doc, err := goquery.NewDocument("https://ubccsss.org/services/exams/")
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		var examPages []string
		doc.Find("a").Each(func(_ int, s *goquery.Selection) {
			href := s.AttrOr("href", "")
			if strings.Contains(href, "exams/cpsc") {
				examPages = append(examPages, "https://ubccsss.org"+href)
			}
		})

		for _, page := range examPages {
			courseCode := strings.ToLower("cs" + path.Base(page)[4:])
			fmt.Fprintf(w, "Loading %s: %s ...\n", courseCode, page)
			doc, err := goquery.NewDocument(page)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			var year int
			doc.Find("article.node h2, article.node a").Each(func(_ int, s *goquery.Selection) {
				tag := s.Get(0).Data
				switch tag {
				case "h2":
					text := strings.Split(s.Text(), " ")[0]
					year, err = strconv.Atoi(text)
					if err != nil {
						http.Error(w, err.Error(), 500)
						return
					}
				case "a":
					href := s.AttrOr("href", "")
					if !strings.Contains(href, "http") {
						href = "https://ubccsss.org/" + href
					}
					if strings.Contains(href, "/files/") {
						fmt.Fprintf(w, "file: %d, %s, %s\n", year, s.Text(), href)
						fetchFileAndSave(w, courseCode, year, s.Text(), href)
					}
				}
			})
		}

		if err := saveAndGenerate(); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		fmt.Fprintf(w, "Done.")
	})

	log.Println("Listening...")
	log.Fatal(http.ListenAndServe("0.0.0.0:8080", nil))
}

func fetchFileAndSave(w http.ResponseWriter, course string, year int, name, href string) {
	resp, err := http.Get(href)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer resp.Body.Close()
	base := path.Base(href)
	dir := fmt.Sprintf("%s/%s/%d", examsDir, course, year)
	if err := os.MkdirAll(dir, 0755); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	raw, _ := ioutil.ReadAll(resp.Body)
	file := path.Join(dir, base)
	if err := ioutil.WriteFile(file, raw, 0755); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	db.addFile(course, year, name, file)
}
