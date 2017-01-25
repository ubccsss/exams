package main

import (
	"fmt"
	"log"
	"net/http"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ubccsss/exams/config"
	"github.com/ubccsss/exams/examdb"
	"github.com/ubccsss/exams/ml"
)

func renderAdminHeader(w http.ResponseWriter) {
	if err := templates.ExecuteTemplate(w, "adminhead.html", nil); err != nil {
		handleErr(w, err)
		return
	}
}

func handlePotentialFileIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	renderAdminHeader(w)

	notAnExam := 0
	for _, f := range db.PotentialFiles {
		if f.NotAnExam {
			notAnExam++
		}
	}

	fmt.Fprintf(w, "<p>Unprocessed files: %d, Processed: %d, Not An Exam: %d</p>", len(db.PotentialFiles)-notAnExam, db.ProcessedCount()+notAnExam, notAnExam)
	w.Write([]byte(`
		<a href="/admin/potential">Unprocessed</a>
		<a href="/admin/potential?invalid">Not Exams/Invalid</a>`))

	_, showInvalid := r.URL.Query()["invalid"]

	if !showInvalid {
		fmt.Fprint(w, "<h1>Unprocessed</h1><ul>")
		for _, file := range db.PotentialFiles {
			if !file.NotAnExam {
				fmt.Fprintf(w, `<li><a href="/admin/file/%s">%s %s</a> %.0f</li>`, file.Hash, file.Source, file.Path, file.Score)
			}
		}
		fmt.Fprint(w, "</ul>")
	} else {
		fmt.Fprint(w, "<h1>Not Exams/Invalid</h1><ul>")
		for _, file := range db.PotentialFiles {
			if file.NotAnExam {
				fmt.Fprintf(w, `<li><a href="/admin/file/%s">%s %s</a></li>`, file.Hash, file.Source, file.Path)
			}
		}
		fmt.Fprint(w, "</ul>")
	}
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
		file.Year = year
		file.Course = course
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
		QuickNames   []string
		Terms        []string
		Term         string
		FileURL      string
		DetectedName string
		DetectedTerm string
	}{
		File:       file,
		Courses:    db.Courses,
		Terms:      examdb.ExamTerms,
		QuickNames: examdb.ExamLabels,
		FileURL:    file.Source,
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

	meta.Year = strconv.Itoa(ml.ExtractYear(file))

	classes, err := ml.DefaultGoogleClassifier.Classify(file, false)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	meta.DetectedName = labelsToName(classes["type"], classes["sample"], classes["solution"])
	meta.DetectedTerm = classes["term"]

	if len(meta.Term) == 0 {
		meta.Term = meta.DetectedTerm
	}

	if err := templates.ExecuteTemplate(w, "file.html", meta); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
}

func handleAdminRemove404(w http.ResponseWriter, r *http.Request) {
	const workers = 8

	fileChan := make(chan *examdb.File)

	go func() {
		files := make([]*examdb.File, len(db.PotentialFiles))
		copy(files, db.PotentialFiles)
		for _, f := range files {
			fileChan <- f
		}
		close(fileChan)
	}()

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			for f := range fileChan {
				if len(f.Path) > 0 {
					continue
				}
				isExamsCGI := strings.HasPrefix(f.Source, "https://www.ugrad.cs.ubc.ca/~q7w9a/exams.cgi/exams.cgi")
				if f.LastResponseCode == 403 && isExamsCGI {
					// Remove 403ed exams.cgi endpoints.
				} else if !(f.LastResponseCode == 404 || f.LastResponseCode == 0) {
					continue
				}

				reader, err := f.Reader()
				if err != nil {
					is404 := strings.Contains(err.Error(), "got 404")
					is403 := strings.Contains(err.Error(), "got 403")
					remove := is404 || isExamsCGI && is403
					fmt.Fprintf(w, "%s: %s (Removing %t)\n", f, err, remove)
					if remove {
						if err := db.RemoveFile(f); err != nil {
							handleErr(w, err)
						}
					}
					continue
				}
				reader.Close()
			}
			wg.Done()
		}()
	}
	wg.Wait()
	w.Write([]byte("Done."))
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
	start := time.Now()
	if err := saveAndGenerate(); err != nil {
		handleErr(w, err)
		return
	}
	fmt.Fprintf(w, "Done in %s.", time.Now().Sub(start))
}

func handleAdminIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	renderAdminHeader(w)
	if err := templates.ExecuteTemplate(w, "admin.html", nil); err != nil {
		handleErr(w, err)
		return
	}
}

func handleNeedFixFileIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	renderAdminHeader(w)

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

func handleMLGoogleAccuracy(w http.ResponseWriter, r *http.Request) {
	if err := ml.DefaultGoogleClassifier.ReportAccuracy(w); err != nil {
		handleErr(w, err)
		return
	}
}

func skipInfer(f *examdb.File) bool {
	return f.NotAnExam || (f.Inferred != nil && (len(f.Inferred.Name) > 0 || f.Inferred.NotAnExam)) || (f.LastResponseCode != 200 && f.LastResponseCode != 0)
}

func handleMLGoogleInferPotential(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Google Prediction Inferring")
	log.Printf("Google Prediction Inferring")
	processed := 0
	for i, f := range db.PotentialFiles {
		if skipInfer(f) {
			continue
		}

		classes, err := ml.DefaultGoogleClassifier.Classify(f, true)
		if err != nil {
			fmt.Fprintf(w, "%s: %s\n", f, err)
			log.Printf("%s: %s\n", f, err)
			continue
		}

		inferred := &examdb.File{
			Name:      labelsToName(classes["type"], classes["sample"], classes["solution"]),
			Term:      classes["term"],
			NotAnExam: classes["isexam"] == ml.IsNotExam,
			Year:      ml.ExtractYear(f),
			Course:    ml.ExtractCourse(&db, f),
		}

		log.Printf("%d: inferred %#v", i, inferred)

		f.Inferred = inferred

		processed++
		if processed%100 == 0 {
			fmt.Fprintf(w, "... processed %d files\n", processed)
			log.Printf("... processed %d files", processed)
		}
	}
	if err := saveAndGenerate(); err != nil {
		handleErr(w, err)
		return
	}
}

func handleMLRetrainGoogle(w http.ResponseWriter, r *http.Request) {
	model, err := ml.MakeGoogleClassifier()
	if err != nil {
		handleErr(w, err)
		return
	}
	if err := model.Train(&db); err != nil {
		handleErr(w, err)
		return
	}
	w.Write([]byte("Done."))
}

func handleErr(w http.ResponseWriter, err error) {
	http.Error(w, fmt.Sprintf("%+v", err), 500)
}
