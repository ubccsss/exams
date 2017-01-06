package main

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

func handlePotentialFileIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, "<p>Unprocessed files: %d, Processed: %d</p>", len(db.PotentialFiles), db.processedCount())
	fmt.Fprint(w, "<h1>Unprocessed</h1><ul>")
	for _, file := range db.PotentialFiles {
		if !file.NotAnExam {
			fmt.Fprintf(w, `<li><a href="/admin/file/%s">%s</a> %.0f</li>`, file.Hash, file.Source, file.Score)
		}
	}
	fmt.Fprint(w, "</ul>")

	fmt.Fprint(w, "<h1>Not Exams/Invalid</h1><ul>")
	for _, file := range db.PotentialFiles {
		if file.NotAnExam {
			fmt.Fprintf(w, `<li><a href="/admin/file/%s">%s</a></li>`, file.Hash, file.Source)
		}
	}
	fmt.Fprint(w, "</ul>")
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
				return f, -1
			}
		}
	}
	return nil, -1
}

func handleFile(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	hash := parts[len(parts)-1]
	file, filei := db.FindFile(hash)
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
		if err := fetchFileAndSave(course, year, term, name, file.Source); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if filei >= 0 {
			db.PotentialFiles = append(db.PotentialFiles[:filei], db.PotentialFiles[filei+1:]...)
		}
		http.Redirect(w, r, "/admin/potential", 302)
		if err := saveAndGenerate(); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		return
	}
	w.Header().Set("Content-Type", "text/html")
	meta := struct {
		File    *File
		Courses map[string]*Course
		Course  string
		Year    string
		Terms   []string
		Term    string
		Label   string
	}{
		File:    file,
		Courses: db.Courses,
		Terms:   []string{"W1", "W2", "S"},
	}

	lowerPath := strings.ToLower(file.Source)
	for c := range db.Courses {
		if strings.Contains(lowerPath, c) {
			meta.Course = c
		}
	}
	years := yearRegex.FindAllString(lowerPath, -1)
	if len(years) > 0 {
		meta.Year = years[len(years)-1]
	}

	// Don't try to match "S" since it's too generic.
	for _, term := range meta.Terms[:2] {
		if strings.Contains(lowerPath, strings.ToLower(term)) {
			meta.Term = term
			break
		}
	}

	typ, samp, sol, _, err := classifier.Classify(file)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	meta.Label = fmt.Sprintf("%s %s %s", samp, typ, sol)

	if err := templates.ExecuteTemplate(w, "file.html", meta); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
}

func handleGenerate(w http.ResponseWriter, r *http.Request) {
	if err := saveAndGenerate(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
}

var indexEndpoints = []string{
	"/admin/generate",
	"/admin/potential",
}

func handleAdminIndex(w http.ResponseWriter, r *http.Request) {
	for _, url := range indexEndpoints {
		fmt.Fprintf(w, `<p><a href="%s">%s</a></p>`, url, url)
	}
}

func handleNeedFixFileIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, `<h1>Files that Potentially Need to be Fixed</h1>
	<table>
	<thead>
	<th>Name</th>
	<th>Path</th>
	<th>Source</th>
	</thead>
	<tbody>`)
	for _, file := range db.needFix() {
		if file.NotAnExam {
			continue
		}
		fmt.Fprintf(w, `<tr><td><a href="/admin/file/%s">%s</a></td><td>%s</td><td>%s</td></tr>`, file.Hash, file.Name, file.Path, file.Source)
	}
	fmt.Fprint(w, `</tbody></table>`)
}

func validFileName(name string) bool {
	for _, label := range labels {
		if name == label {
			return true
		}
	}
	return false
}

func (db Database) needFix() []*File {
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
