package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/d4l3k/exams/archive.org"
	"github.com/d4l3k/exams/examdb"
	"github.com/urfave/cli"
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
		db.AddCourse(w, string(part), "")
	}

	doc, err := goquery.NewDocument("https://courses.students.ubc.ca/cs/main?dept=CPSC&pname=subjarea&req=1&tname=subjareas")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	doc.Find("#mainTable tr").Each(func(_ int, s *goquery.Selection) {
		tds := s.Find("td")
		link := tds.Find("a")
		linkTitle := strings.ToLower(strings.TrimSpace(link.Text()))
		if strings.HasPrefix(linkTitle, "cpsc ") {
			course := "cs" + linkTitle[5:]
			desc := strings.TrimSpace(tds.Eq(1).Text())
			db.AddCourse(w, course, desc)
		}
	})

	fmt.Fprintf(w, "Done.")

	if err := saveAndGenerate(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
}

// ingressDeptFiles talks to the exams.cgi binary running on the ugrad servers and
// returns potential file matches.
func ingressDeptFiles(w http.ResponseWriter, r *http.Request) {
	req, err := http.Get("https://www.ugrad.cs.ubc.ca/~q7w9a/exams.cgi/exams.cgi/")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	var files []*examdb.File
	if err := json.NewDecoder(req.Body).Decode(&files); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	for _, f := range files {
		f.Source = f.Path
		f.Path = ""
	}
	db.AddPotentialFiles(w, files)
	if err := saveAndGenerate(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	fmt.Fprintf(w, "Done.")
}

func ingressArchiveOrgFiles(w http.ResponseWriter, r *http.Request) {
	urls := examsarchiveorg.PossibleExams()
	var files []*examdb.File
	for _, u := range urls {
		files = append(files, &examdb.File{
			Source: u,
		})
	}
	db.AddPotentialFiles(w, files)
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
					f := examdb.File{
						Course: courseCode,
						Year:   year,
						Name:   s.Text(),
						Source: href,
					}
					if err := db.FetchFileAndSave(&f); err != nil {
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

func ingressPotentialFile(c *cli.Context) {
	file := c.String("file")

	if len(file) == 0 {
		log.Fatal("need to provide file")
	}

	var reader io.ReadCloser
	var err error
	if file == "-" {
		reader = os.Stdin
	} else {
		reader, err = os.Open(file)
		if err != nil {
			log.Fatal(err)
		}
	}
	defer reader.Close()

	var files []*examdb.File
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		url := scanner.Text()
		f := &examdb.File{
			Source: url,
		}
		files = append(files, f)
	}
	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}

	db.AddPotentialFiles(os.Stderr, files)
	if err := saveAndGenerate(); err != nil {
		log.Fatal(err)
	}
	log.Println("Done.")
}

func setupIngressCommands() cli.Command {
	return cli.Command{
		Name:    "ingress",
		Aliases: []string{"i"},
		Subcommands: []cli.Command{
			{
				Name:   "potential",
				Usage:  "import a bunch of potential files via a file",
				Action: ingressPotentialFile,
				Flags: []cli.Flag{
					cli.StringFlag{
						Name:  "file, f",
						Usage: "Load potential files from `FILE`",
					},
				},
			},
		},
	}
}
