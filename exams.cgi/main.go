package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/cgi"
	"path"
	"strings"
)

type file struct {
	Path string
	Hash string
}

func files() ([]file, error) {
	raw, err := ioutil.ReadFile("../exam_shas1.txt")
	if err != nil {
		return nil, err
	}
	var files []file
	for _, line := range bytes.Split(raw, []byte("\n")) {
		pieces := bytes.Split(line, []byte(" "))
		if len(pieces) < 2 {
			continue
		}
		files = append(files, file{
			Path: string(bytes.Join(pieces[1:], nil)),
			Hash: string(pieces[0]),
		})
	}
	return files, nil
}

func listing(w http.ResponseWriter, base string) error {
	files, err := files()
	if err != nil {
		return err
	}
	for i, file := range files {
		files[i].Path = "https://www.ugrad.cs.ubc.ca" + path.Join(base, file.Path)
	}
	if err := json.NewEncoder(w).Encode(files); err != nil {
		return err
	}
	return nil
}

func main() {
	if err := cgi.Serve(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		parts := strings.Split(r.URL.Path, ".cgi")
		url := parts[len(parts)-1]
		if url == "/" {
			header.Set("Content-Type", "application/json; charset=utf-8")
			if err := listing(w, r.URL.Path); err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
		} else {
			files, err := files()
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			for _, file := range files {
				if file.Path == url {
					http.ServeFile(w, r, file.Path)
					return
				}
			}
			http.Error(w, "file not found", 404)
		}
	})); err != nil {
		fmt.Println(err)
	}
}
