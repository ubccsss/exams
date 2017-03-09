package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/ubccsss/exams/archive.org"
	"github.com/ubccsss/exams/config"
	"github.com/ubccsss/exams/examdb"
	"github.com/ubccsss/exams/ml"
	"github.com/ubccsss/exams/workers"
	"github.com/urfave/cli"
)

// ingressDeptCourses sshes into the dept server and fetches the courses.
func ingressDeptCourses(w http.ResponseWriter, r *http.Request) {

	fmt.Fprintf(w, "Fetching from ugrad servers...\n")

	resp, err := exec.Command("ssh", "q7w9a@remote.ugrad.cs.ubc.ca", "-C", "ls /home/c").Output()
	if err != nil {
		fmt.Fprintf(w, "%+v\n", err)
	} else {
		parts := bytes.Split(resp, []byte("\n"))
		courseRegex := regexp.MustCompile("^cs\\d{3}$")
		for _, part := range parts {
			if !courseRegex.Match(part) {
				continue
			}
			db.AddCourse(w, string(part), "")
		}
	}

	for _, dept := range config.Departments {
		fmt.Fprintf(w, "Fetching courses for: %s\n", dept)
		fmt.Fprintf(w, "Fetching from courses.students.ubc.ca...\n")

		coursesURL := fmt.Sprintf("https://courses.students.ubc.ca/cs/main?dept=%s&pname=subjarea&req=1&tname=subjareas", dept)
		doc, err := goquery.NewDocument(coursesURL)
		if err != nil {
			fmt.Fprintf(w, "%+v\n", err)
		} else {
			doc.Find("#mainTable tr").Each(func(_ int, s *goquery.Selection) {
				tds := s.Find("td")
				link := tds.Find("a")
				linkTitle := strings.ToLower(strings.TrimSpace(link.Text()))
				if strings.HasPrefix(linkTitle, strings.ToLower(dept)+" ") {
					course := linkTitle
					// TODO(d4l3k): Remove special case for CS.
					if dept == config.ComputerScience {
						course = "cs" + linkTitle[5:]
					}
					desc := strings.TrimSpace(tds.Eq(1).Text())
					db.AddCourse(w, course, desc)
				}
			})
		}

		fmt.Fprintf(w, "Fetching from http://www.calendar.ubc.ca/archive/vancouver/...\n")

		coursesRegexp, err := regexp.Compile(fmt.Sprintf(`^%s\s+(\d{3})\s+\(.+\)\s+(\w)?\s+(.+)$`, dept))
		if err != nil {
			fmt.Fprintf(w, "%+v\n", err)
			return
		}

		currentYear := time.Now().Year()
		lastTwoYear := currentYear - (currentYear/100)*100
		for i := lastTwoYear; i >= 2; i-- {
			url := fmt.Sprintf("http://www.calendar.ubc.ca/archive/vancouver/%.2d%.2d/courses.html", i, i+1)
			doc, err := goquery.NewDocument(url)
			if err != nil {
				fmt.Fprintf(w, "%+v\n", err)
				continue
			}

			subjectURL := getURLForLink(doc, "courses by subject code")
			if len(subjectURL) == 0 {
				fmt.Fprintf(w, "failed to find subject page for %q\n", url)
				continue
			}

			subjectDoc, err := goquery.NewDocument(subjectURL)
			if err != nil {
				fmt.Fprintf(w, "%+v\n", err)
				continue
			}

			coursesURL := getURLForLink(subjectDoc, dept)
			if len(coursesURL) == 0 {
				fmt.Fprintf(w, "failed to find courses page for %q", subjectURL)
				continue
			}

			coursesDoc, err := goquery.NewDocument(coursesURL)
			if err != nil {
				fmt.Fprintf(w, "%+v\n", err)
				continue
			}

			coursesDoc.Find("dl > dt").Each(func(_ int, s *goquery.Selection) {
				text := s.Text()
				matches := coursesRegexp.FindStringSubmatch(text)
				if len(matches) != 4 {
					return
				}
				courseNumber := matches[1] + matches[2]
				course := dept + " " + courseNumber
				// TODO(d4l3k): Remove special case for CS.
				if dept == config.ComputerScience {
					course = "cs" + courseNumber
				}
				desc := matches[3]
				fmt.Fprintf(w, "%s: %s\n", course, desc)
				db.AddCourse(w, course, desc)
			})
		}
	}

	fmt.Fprintf(w, "Done.\n")

	if err := saveAndGenerate(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
}

func getURLForLink(doc *goquery.Document, text string) string {
	target := strings.ToLower(strings.TrimSpace(text))
	var found string
	doc.Find("a").Each(func(_ int, s *goquery.Selection) {
		text := strings.ToLower(strings.TrimSpace(s.Text()))
		if text == target {
			found = s.AttrOr("href", "")
		}
	})
	if len(found) == 0 {
		return ""
	}
	u, err := url.Parse(found)
	if err != nil {
		log.Println(err)
		return ""
	}
	return doc.Url.ResolveReference(u).String()
}

// ingressDeptFiles talks to the exams.cgi binary running on the ugrad servers and
// returns potential file matches.
func ingressDeptFiles(w http.ResponseWriter, r *http.Request) {
	req, err := http.Get("https://www.ugrad.cs.ubc.ca/~q7w9a/exams.cgi/")
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
		strippedPath := strings.TrimPrefix(f.Path, "https://www.ugrad.cs.ubc.ca/~q7w9a/exams.cgi")
		url, ok := ugradPathToHTTP(strippedPath)
		if ok {
			f.Source = url
		} else {
			f.Source = f.Path
		}
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

var matchExamTitleRegexp = regexp.MustCompile(`^(\d{4})(WT1|WT2|S)(\(sec.*\))?$`)

// ingressUBCMath ingresses the exams on the math department's site.
func ingressUBCMath(w http.ResponseWriter, r *http.Request) {
	doc, err := goquery.NewDocument("https://www.math.ubc.ca/Ugrad/pastExams/")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	filesChan := make(chan *examdb.File, workers.Count)
	var wg sync.WaitGroup

	for i := 0; i < workers.Count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for f := range filesChan {
				if err := db.FetchFileAndSave(f); err != nil {
					fmt.Fprintf(w, "%s: %+v\n", f, err)
				}
			}
		}()
	}

	doc.Find("#main table[align=center] tr").Each(func(_ int, s *goquery.Selection) {
		childrenCount := s.Children().Length()
		if childrenCount != 2 {
			return
		}

		parts := strings.Split(s.Find("th").Text(), "+")
		if len(parts) == 0 {
			return
		}

		code := strings.ToLower(fmt.Sprintf("%s %s", config.Math, parts[0]))

		s.Find("td a[href]").Each(func(_ int, s *goquery.Selection) {
			matches := matchExamTitleRegexp.FindStringSubmatch(s.Text())
			if len(matches) < 3 {
				return
			}
			year, _ := ml.ExtractYearFromWords(matches)
			var term string
			switch matches[2] {
			case "WT1":
				term = examdb.TermW1
			case "WT2":
				term = examdb.TermW2
			case "S":
				term = examdb.TermS
			default:
				log.Fatalf("invalid term: %#v", matches)
			}

			u, err := url.Parse(s.AttrOr("href", ""))
			if err != nil {
				fmt.Fprintf(w, "%+v\n", err)
				return
			}
			absURL := doc.Url.ResolveReference(u).String()

			f := examdb.File{
				Course:         code,
				Year:           year,
				Term:           term,
				Name:           "Final",
				Source:         absURL,
				HandClassified: true,
			}

			fmt.Fprintf(w, "%#v\n", f)
			filesChan <- &f
		})
	})

	close(filesChan)
	wg.Wait()

	if err := saveAndGenerate(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	fmt.Fprintf(w, "Done.")
}

// ingressUBCLaw ingresses the exams on the law department's site.
func ingressUBCLaw(w http.ResponseWriter, r *http.Request) {
	doc, err := goquery.NewDocument("http://law.library.ubc.ca/exams/")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	filesChan := make(chan *examdb.File, workers.Count)
	var wg sync.WaitGroup

	for i := 0; i < workers.Count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for f := range filesChan {
				if err := db.FetchFileAndSave(f); err != nil {
					fmt.Fprintf(w, "%s: %+v\n", f, err)
				}
			}
		}()
	}

	var pages []string
	doc.Find(".entry-content a[href]").Each(func(_ int, s *goquery.Selection) {
		u, err := url.Parse(s.AttrOr("href", ""))
		if err != nil {
			fmt.Fprintf(w, "%+v\n", err)
			return
		}
		absURL := doc.Url.ResolveReference(u).String()
		pages = append(pages, absURL)
	})

	for _, page := range pages {
		doc, err := goquery.NewDocument(page)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		date := strings.Split(doc.Find("h1").Text(), " – ")[0]
		t, err := time.Parse("2006 January", date)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		year, term := ml.ConvertDateToYearTerm(t)

		doc.Find(".entry-content table a[href]").Each(func(_ int, s *goquery.Selection) {
			code := strings.TrimSpace(strings.ToLower(strings.Split(s.Text(), ":")[0]))
			u, err := url.Parse(s.AttrOr("href", ""))
			if err != nil {
				fmt.Fprintf(w, "%+v\n", err)
				return
			}
			absURL := doc.Url.ResolveReference(u).String()

			f := examdb.File{
				Course:         code,
				Year:           year,
				Term:           term,
				Name:           "Final",
				Source:         absURL,
				HandClassified: true,
			}

			fmt.Fprintf(w, "%#v\n", f)
			filesChan <- &f
		})
	}

	close(filesChan)
	wg.Wait()

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
