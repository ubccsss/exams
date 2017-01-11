package main

import (
	"fmt"
	"net/http"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/ubccsss/exams/config"
	"github.com/ubccsss/exams/examdb"
	"github.com/ubccsss/exams/ml"
	"github.com/ubccsss/exams/util"
)

func handlePotentialFileIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, "<p>Unprocessed files: %d, Processed: %d</p>", len(db.PotentialFiles), db.ProcessedCount())
	fmt.Fprint(w, "<h1>Unprocessed</h1><ul>")
	for _, file := range db.PotentialFiles {
		if !file.NotAnExam {
			fmt.Fprintf(w, `<li><a href="/admin/file/%s">%s %s</a> %.0f</li>`, file.Hash, file.Source, file.Path, file.Score)
		}
	}
	fmt.Fprint(w, "</ul>")

	fmt.Fprint(w, "<h1>Not Exams/Invalid</h1><ul>")
	for _, file := range db.PotentialFiles {
		if file.NotAnExam {
			fmt.Fprintf(w, `<li><a href="/admin/file/%s">%s %s</a></li>`, file.Hash, file.Source, file.Path)
		}
	}
	fmt.Fprint(w, "</ul>")
}

func handleFile(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	hash := parts[len(parts)-1]
	file := db.FindFile(hash)
	if file == nil {
		http.Error(w, "not found", 404)
		return
	}

	if r.Method == "POST" {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if len(r.FormValue("invalid")) > 0 {
			file.NotAnExam = true
			http.Redirect(w, r, "/admin/potential", 302)
			if err := saveAndGenerate(); err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			return
		}
		course := r.FormValue("course")
		if len(course) == 0 {
			http.Error(w, "must specify course", 400)
			return
		}
		name := r.FormValue("name")
		quickname := r.FormValue("quickname")
		if len(name) > 0 && len(quickname) > 0 {
			http.Error(w, "can't have both name and quickname", 400)
			return
		}
		name += quickname
		if len(name) == 0 {
			http.Error(w, "must specify name", 400)
			return
		}
		term := r.FormValue("term")
		year, err := strconv.Atoi(r.FormValue("year"))
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		file.Course = course
		file.Year = year
		file.Term = term
		file.Name = name
		if err := db.RemoveFile(file); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if err := db.FetchFileAndSave(file); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if redirectParam, ok := r.URL.Query()["redirect"]; ok && len(redirectParam) > 0 {
			http.Redirect(w, r, redirectParam[0], 302)
		} else {
			http.Redirect(w, r, "/admin/potential", 302)
		}
		if err := saveAndGenerate(); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		return
	}
	w.Header().Set("Content-Type", "text/html")
	meta := struct {
		File         *examdb.File
		Courses      map[string]*examdb.Course
		Course       string
		Year         string
		Terms        []string
		Term         string
		FileURL      string
		DetectedName string
		DetectedTerm string
	}{
		File:    file,
		Courses: db.Courses,
		Terms:   []string{"W1", "W2", "S"},
		FileURL: file.Source,
	}

	if len(meta.FileURL) == 0 {
		meta.FileURL = path.Join("/", file.Path)
	}
	if file.Year > 0 {
		meta.Year = strconv.Itoa(file.Year)
	}
	if len(file.Term) > 0 {
		meta.Term = file.Term
	}
	if len(file.Course) > 0 {
		meta.Course = file.Course
	}

	if len(meta.Course) == 0 {
		meta.Course = ml.ExtractCourse(&db, file)
	}

	lowerPath := strings.ToLower(file.Source)
	years := util.YearRegexp.FindAllString(lowerPath, -1)
	if len(years) > 0 {
		meta.Year = years[len(years)-1]
	}

	typ, samp, sol, termClass, err := ml.Classifier.Classify(file)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	meta.DetectedName = labelsToName(string(typ), string(samp), string(sol))
	meta.DetectedTerm = string(termClass)

	if len(meta.Term) == 0 {
		meta.Term = meta.DetectedTerm
	}

	if err := templates.ExecuteTemplate(w, "file.html", meta); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
}

func labelsToName(typ, samp, sol string) string {
	var bits []string
	for _, class := range []string{samp, typ} {
		if len(class) == 0 {
			continue
		}
		bits = append(bits, class)
	}
	if len(sol) > 0 {
		bits = append(bits, "(Solution)")
	}
	return strings.Join(bits, " ")
}

func handleGenerate(w http.ResponseWriter, r *http.Request) {
	if err := saveAndGenerate(); err != nil {
		http.Error(w, fmt.Sprintf("%+v", err), 500)
		return
	}
	w.Write([]byte("Done."))
}

var indexEndpoints = []string{
	"/admin/generate",
	"/admin/potential",
	"/admin/needfix",
	"/admin/ml/retrain",
}

func handleAdminIndex(w http.ResponseWriter, r *http.Request) {
	for _, url := range indexEndpoints {
		fmt.Fprintf(w, `<p><a href="%s">%s</a></p>`, url, url)
	}
}

func handleNeedFixFileIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	files := db.NeedFix()
	fmt.Fprintf(w, `<h1>Files that Potentially Need to be Fixed (%d)</h1>
	<table>
	<thead>
	<th>Name</th>
	<th>Path</th>
	<th>Source</th>
	</thead>
	<tbody>`, len(files))
	sort.Sort(examdb.FileByName(files))
	for _, file := range files {
		if file.NotAnExam {
			continue
		}
		fmt.Fprintf(w, `<tr><td><a href="/admin/file/%s?redirect=/admin/needfix">%s</a></td><td>%s</td><td>%s</td></tr>`, file.Hash, file.Name, file.Path, file.Source)
	}
	fmt.Fprint(w, `</tbody></table>`)
}

func handleMLRetrain(w http.ResponseWriter, r *http.Request) {
	if err := ml.RetrainClassifier(&db, config.ClassifierDir); err != nil {
		handleErr(w, err)
		return
	}
	w.Write([]byte("Done."))
}

func handleErr(w http.ResponseWriter, err error) {
	http.Error(w, fmt.Sprintf("%+v", err), 500)
}
