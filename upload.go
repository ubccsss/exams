package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strconv"

	"github.com/ubccsss/exams/config"
	"github.com/ubccsss/exams/examdb"
)

func handleFileUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		handleErr(w, errors.New("POST required"))
		return
	}
	if r.ContentLength > config.MaxFileSize {
		http.Error(w, "request too large", http.StatusExpectationFailed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, config.MaxFileSize)
	if err := r.ParseMultipartForm(1024); err != nil {
		handleErr(w, err)
		return
	}

	// Antispam
	if r.FormValue("shouldbeempty") != "" {
		return
	}

	course := r.URL.Query().Get("course")
	if _, ok := db.Courses[course]; !ok {
		http.Error(w, "invalid course ID", http.StatusBadRequest)
		return
	}

	name := r.FormValue("name")
	if name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}
	year := r.FormValue("year")
	if year == "" {
		http.Error(w, "year required", http.StatusBadRequest)
		return
	}
	yeari, err := strconv.Atoi(year)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	term := r.FormValue("term")
	if term == "" {
		http.Error(w, "term required", http.StatusBadRequest)
		return
	}

	file, handler, err := r.FormFile("exam")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()
	fpath := path.Join(config.UploadedExamsDir, handler.Filename)
	if err := os.MkdirAll(path.Join(config.ExamsDir, config.UploadedExamsDir), 0755); err != nil {
		handleErr(w, err)
		return
	}
	f, err := os.OpenFile(path.Join(config.ExamsDir, fpath), os.O_WRONLY|os.O_CREATE, 0755)
	if err != nil {
		handleErr(w, err)
		return
	}
	defer f.Close()
	if _, err := io.Copy(f, file); err != nil {
		handleErr(w, err)
		return
	}

	db.AddPotentialFiles(os.Stderr, []*examdb.File{{
		Name:   name,
		Course: course,
		Year:   yeari,
		Term:   term,
		Path:   fpath,
	}})

	fmt.Fprintf(w, `<h1>Uploaded Successful</h1>
	<p>Thank you for your contribution!</p>
	<a href="/%s/">Return to %s</a>.`,
		course, course)
}
