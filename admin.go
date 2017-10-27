package main

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ubccsss/exams/db"
	"github.com/ubccsss/exams/examdb"
	"github.com/ubccsss/exams/generators"
)

// Mappings to the new locations of the functions.
var (
	renderAdminHeader = generators.RenderAdminHeader
	handleErr         = generators.HandleErr
)

// adminRoutes returns a mux for all of the admin endpoints.
func (s *server) adminRoutes() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/admin/potential", s.handlePotentialFileIndex)
	//mux.HandleFunc("/admin/needfix", s.handleNeedFixFileIndex)
	mux.HandleFunc("/admin/file/", s.handleFile)

	/*
		mux.HandleFunc("/admin/duplicates", generators.PrettyJob(s.handleListDuplicates))
		mux.HandleFunc("/admin/removeDuplicates", generators.PrettyJob(s.handleRemoveDuplicates))
		mux.HandleFunc("/admin/incorrectlocations", generators.PrettyJob(s.handleListIncorrectLocations))
	*/

	// Machine Learning Endpoints
	/*
		mux.HandleFunc("/admin/ml/bayesian/train", generators.PrettyJob(s.handleMLRetrain))
		mux.HandleFunc("/admin/ml/google/train", generators.PrettyJob(s.handleMLRetrainGoogle))
		mux.HandleFunc("/admin/ml/google/inferpotential", generators.PrettyJob(s.handleMLGoogleInferPotential))
		mux.HandleFunc("/admin/ml/google/accuracy", generators.PrettyJob(s.handleMLGoogleAccuracy))
	*/

	// Ingress Endpoints
	/*
		mux.HandleFunc("/admin/ingress/deptcourses", generators.PrettyJob(ingressDeptCourses))
		mux.HandleFunc("/admin/ingress/deptfiles", generators.PrettyJob(ingressDeptFiles))
		mux.HandleFunc("/admin/ingress/ubccsss", generators.PrettyJob(ingressUBCCSSS))
		mux.HandleFunc("/admin/ingress/ubcmath", generators.PrettyJob(ingressUBCMath))
		mux.HandleFunc("/admin/ingress/ubclaw", generators.PrettyJob(ingressUBCLaw))
		mux.HandleFunc("/admin/ingress/archive.org", generators.PrettyJob(ingressArchiveOrgFiles))
	*/

	mux.HandleFunc("/admin/", s.handleAdminIndex)

	return mux
}

func (s *server) handlePotentialFileIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	renderAdminHeader(w)

	count := s.db.FileCount()

	fmt.Fprintf(w, "<title>Potential Files</title>")

	fmt.Fprintf(w, "<p>Unprocessed files: %d, Processed: %d, Not An Exam: %d, Total: %d</p>", count.Potential, count.HandClassified, count.NotAnExam, count.Total)
	w.Write([]byte(`
		<a href="/admin/potential">Unprocessed</a>,
		<a href="/admin/potential?invalid">Not Exams/Invalid</a>,
		<a href="/admin/potential?inferred">Inferred: Unprocessed</a>,
		<a href="/admin/potential?inferred&invalid">Inferred: Not Exams/Invalid</a>
		`))

	files, err := s.db.Files(db.File{})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	fmt.Fprint(w, "<h1>All Files</h1><ul>")
	for _, file := range files {
		fmt.Fprintf(w, `<li><a href="/admin/file/%s">%s %s</a></li>`, file.Hash, file.SourceURL, file.URL)
	}
	fmt.Fprint(w, "</ul>")

	/*
		_, showInvalid := r.URL.Query()["invalid"]
		_, showInferred := r.URL.Query()["inferred"]

		if showInvalid && !showInferred {
			fmt.Fprint(w, "<h1>Not Exams/Invalid</h1><ul>")
			files, err := s.db.NotAnExamFiles()
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			for _, file := range files {
				fmt.Fprintf(w, `<li><a href="/admin/file/%s">%s %s</a></li>`, file.Hash, file.SourceURL, file.URL)
			}
			fmt.Fprint(w, "</ul>")
		} else if showInvalid && showInferred {
			fmt.Fprint(w, "<h1>Inferred: Not Exam/Invalid</h1><ul>")
			for _, file := range s.db.Files {
				if file.Inferred == nil || !file.Inferred.NotAnExam || file.HandClassified {
					continue
				}
				fmt.Fprintf(w, `<li><a href="/admin/file/%s">%s %s</a></li>`, file.Hash, file.Source, file.Path)
			}
			fmt.Fprint(w, "</ul>")
		} else if !showInvalid && showInferred {
			fmt.Fprint(w, "<h1>Inferred: Unprocessed</h1><ul>")
			for _, file := range db.Files {
				if file.Inferred == nil || file.Inferred.NotAnExam || file.HandClassified {
					continue
				}
				fmt.Fprintf(w, `<li><a href="/admin/file/%s">%s %s</a></li>`, file.Hash, file.Source, file.Path)
			}
			fmt.Fprint(w, "</ul>")
		} else {
			fmt.Fprint(w, "<h1>Unprocessed</h1><ul>")
			for _, file := range db.UnprocessedFiles() {
				fmt.Fprintf(w, `<li><a href="/admin/file/%s">%s %s</a> %.0f</li>`, file.Hash, file.Source, file.Path, file.Score)
			}
			fmt.Fprint(w, "</ul>")
		}
	*/
}

func (s *server) handleFilePost(w http.ResponseWriter, r *http.Request, file *db.File) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if len(r.FormValue("invalid")) > 0 {
		file.NotAnExam = true
		file.HandClassified = true
		http.Redirect(w, r, "/admin/potential", 302)
		return
	}
	courseID := r.FormValue("course")
	if len(courseID) == 0 {
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

	course, err := s.db.Course(courseID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	file.CourseFaculty = course.Faculty
	file.CourseCode = course.Code

	file.Term = term
	file.Label = name
	file.HandClassified = true
	if err := s.db.SaveFile(file); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if redirectParam, ok := r.URL.Query()["redirect"]; ok && len(redirectParam) > 0 {
		http.Redirect(w, r, redirectParam[0], 302)
	} else {
		http.Redirect(w, r, "/admin/potential", 302)
	}
}

func (s *server) handleFile(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	hash := parts[len(parts)-1]
	file, err := s.db.File(hash)
	if err != nil {
		handleErr(w, err)
		return
	}

	if r.Method == "POST" {
		s.handleFilePost(w, r, &file)
		return
	}

	w.Header().Set("Content-Type", "text/html")

	displayCourses, err := s.db.DisplayCourses()
	if err != nil {
		handleErr(w, err)
		return
	}

	meta := struct {
		File         db.File
		Courses      []string
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
		Courses:    displayCourses,
		Terms:      examdb.ExamTerms,
		QuickNames: examdb.ExamLabels,
		FileURL:    file.SourceURL,
	}

	if len(file.URL) > 0 {
		meta.FileURL = file.URL
	}
	if file.Year > 0 {
		meta.Year = strconv.Itoa(file.Year)
	}
	if len(file.Term) > 0 {
		meta.Term = file.Term
	}
	course := db.Course{Faculty: file.CourseFaculty, Code: file.CourseCode}.Title()
	if len(course) > 0 {
		meta.Course = course
	}

	/*
		if len(meta.Course) == 0 {
			meta.Course = ml.ExtractCourse(&db, file)
		}
	*/

	//year, _ := ml.ExtractYear(file)
	year := 0
	meta.Year = strconv.Itoa(year)

	/*
		classes, err := ml.DefaultGoogleClassifier.Classify(file, false)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		meta.DetectedName = labelsToName(classes["type"], classes["sample"], classes["solution"])
		meta.DetectedTerm = classes["term"]
	*/

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

func (s *server) handleAdminIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	renderAdminHeader(w)
	if err := generators.ExecuteTemplate(w, "admin.md", nil); err != nil {
		handleErr(w, err)
		return
	}
}

/*
func (s *server) handleNeedFixFileIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	renderAdminHeader(w)

	reasons := db.NeedFix()
	fmt.Fprintf(w, `<h1>Files that Potentially Need to be Fixed (%d)</h1>
	<table>
	<thead>
	<th>Name</th>
	<th>Reasons</th>
	<th>Path</th>
	<th>Source</th>
	</thead>
	<tbody>`, len(reasons))
	sort.Slice(reasons, func(i int, j int) bool {
		return strings.ToLower(reasons[i].File.Name) < strings.ToLower(reasons[j].File.Name)
	})
	for _, reason := range reasons {
		file := reason.File
		if file.NotAnExam {
			continue
		}
		fmt.Fprintf(w, `<tr>
		<td><a href="/admin/file/%s?redirect=/admin/needfix">%s</a></td>
		<td>%s</td>
		<td>%s</td>
		<td>%s</td>
		</tr>`, file.Hash, file.Name, strings.Join(reason.Reasons, ", "), file.Path, file.Source)
	}
	fmt.Fprint(w, `</tbody></table>`)
}
*/

/*
func (s *server) handleMLRetrain(w http.ResponseWriter, r *http.Request) {
	if err := ml.RetrainClassifier(&db, config.ClassifierDir); err != nil {
		handleErr(w, err)
		return
	}
	w.Write([]byte("Done."))
}

func (s *server) handleMLGoogleAccuracy(w http.ResponseWriter, r *http.Request) {
	if err := ml.DefaultGoogleClassifier.ReportAccuracy(w); err != nil {
		handleErr(w, err)
		return
	}
}
*/

// If always is true, things will be inferred if more than 24 hours old.
func skipInfer(f *examdb.File, always bool) bool {
	should := always && f.Inferred != nil && time.Since(f.Inferred.Updated) > 24*time.Hour
	return f.NotAnExam || f.HandClassified || (f.Inferred != nil && (len(f.Inferred.Name) > 0 || f.Inferred.NotAnExam) && !should) || (f.LastResponseCode != 200 && f.LastResponseCode != 0)
}

/*
func (s *server) handleMLGoogleInferPotential(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Google Prediction Inferring\n")

	alwaysInfer := r.URL.RawQuery == "alwaysinfer"
	if alwaysInfer {
		fmt.Fprintf(w, "NOTE: always inferring, very expensive (for update times > 1day ago)\n")
	}

	type fileIndex struct {
		i    int
		file *examdb.File
	}
	fileChan := make(chan fileIndex, workers.Count)

	processed := 0
	go func() {
		for i, f := range db.UnprocessedFiles() {
			if skipInfer(f, alwaysInfer) {
				continue
			}

			fileChan <- fileIndex{i, f}
		}
		close(fileChan)
	}()

	var wg sync.WaitGroup
	for i := 0; i < workers.Count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for fi := range fileChan {
				f := fi.file
				classes, err := ml.DefaultGoogleClassifier.Classify(f, true)
				if err != nil {
					fmt.Fprintf(w, "%s: %s\n", f, err)
					continue
				}

				year, _ := ml.ExtractYear(f)
				inferred := &examdb.File{
					Name:      labelsToName(classes["type"], classes["sample"], classes["solution"]),
					Term:      classes["term"],
					NotAnExam: classes["isexam"] == ml.IsNotExam,
					Year:      year,
					Course:    ml.ExtractCourse(&db, f),
					Updated:   time.Now(),
				}

				fmt.Fprintf(w, "%d. inferred %#v\n", i, inferred)

				f.Inferred = inferred

				processed++
				if processed%100 == 0 {
					fmt.Fprintf(w, "... processed %d files\n", processed)
				}
			}
		}()
	}
	wg.Wait()

	if err := saveAndGenerate(); err != nil {
		handleErr(w, err)
		return
	}
}

func (s *server) handleMLRetrainGoogle(w http.ResponseWriter, r *http.Request) {
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
*/
