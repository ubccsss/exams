package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/d4l3k/exams/archive.org"
)

// ingressDeptCourses sshes into the dept server and fetches the courses.
func ingressDeptCourses(w http.ResponseWriter, r *http.Request) {
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
		db.AddCourse(w, string(part))
	}

	doc, err := goquery.NewDocument("https://courses.students.ubc.ca/cs/main?dept=CPSC&pname=subjarea&req=1&tname=subjareas")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	doc.Find("#mainTable a").Each(func(_ int, s *goquery.Selection) {
		linkTitle := strings.ToLower(strings.TrimSpace(s.Text()))
		if strings.HasPrefix(linkTitle, "cpsc ") {
			course := "cs" + linkTitle[5:]
			db.AddCourse(w, course)
		}
	})

	fmt.Fprintf(w, "Done.")

	if err := saveAndGenerate(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
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

// ingressDeptFiles talks to the exams.cgi binary running on the ugrad servers and
// returns potential file matches.
func ingressDeptFiles(w http.ResponseWriter, r *http.Request) {
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
	for _, f := range files {
		f.Source = f.Path
		f.Path = ""
	}
	db.addPotentialFiles(w, files)
	if err := saveAndGenerate(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	fmt.Fprintf(w, "Done.")
}

func ingressArchiveOrgFiles(w http.ResponseWriter, r *http.Request) {
	urls := examsarchiveorg.PossibleExams()
	var files []*File
	for _, u := range urls {
		files = append(files, &File{
			Source: u,
		})
	}
	db.addPotentialFiles(w, files)
	if err := saveAndGenerate(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	fmt.Fprintf(w, "Done.")
}

// ingressUBCCSSS ingresses the current exams on the website.
func ingressUBCCSSS(w http.ResponseWriter, r *http.Request) {
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
					if err := fetchFileAndSave(courseCode, year, "", s.Text(), href); err != nil {
						http.Error(w, err.Error(), 500)
						return
					}
				}
			}
		})
	}

	if err := saveAndGenerate(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	fmt.Fprintf(w, "Done.")
}
