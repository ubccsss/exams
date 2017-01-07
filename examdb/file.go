package examdb

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/alecthomas/units"
	"github.com/d4l3k/exams/util"
	"github.com/pkg/errors"
)

var (
	// MaxFileSize is the max size of a file that we'll handle.
	MaxFileSize = int64(10 * units.MB)

	// ExamLabels are all the possible labels that a file can fall under.
	ExamLabels = []string{
		"Final",
		"Final (Solution)",
		"Sample Final",
		"Sample Final (Solution)",
		"Midterm",
		"Midterm (Solution)",
		"Sample Midterm",
		"Sample Midterm (Solution)",
		"Midterm 1",
		"Midterm 1 (Solution)",
		"Sample Midterm 1",
		"Sample Midterm 1 (Solution)",
		"Midterm 2",
		"Midterm 2 (Solution)",
		"Sample Midterm 2",
		"Sample Midterm 2 (Solution)",
	}

	// FileNameScoreRegexes are a list of regexps and values that can be used to
	// rank files based on how likely they are an exam.
	FileNameScoreRegexes = map[int][]string{
		1:  []string{"final", "exam", "midterm", "sample", "mt", "(cs|cpsc)\\d{3}", "(20|19)\\d{2}"},
		-1: []string{"report", "presentation", "thesis", "slide"},
	}
)

// File is a single exam file typically a PDF.
type File struct {
	Name      string
	Path      string
	Source    string
	Hash      string
	Score     float64
	Term      string
	NotAnExam bool
}

// Reader opens the file either over HTTP or from disk and returns an
// io.ReadCloser which needs to be closed by the caller.
func (f *File) Reader() (io.ReadCloser, error) {
	var source io.ReadCloser
	if len(f.Path) > 0 {
		var err error
		source, err = os.Open(f.Path)
		if err != nil {
			return nil, err
		}
	} else if len(f.Source) > 0 {
		req, err := http.Get(f.Source)
		if err != nil {
			return nil, err
		}
		source = req.Body
	} else {
		return nil, errors.Errorf("No source or path for %+v", f)
	}
	return source, nil
}

func (f File) String() string {
	return fmt.Sprintf("%s %s %s", f.Name, f.Path, f.Source)
}

// ComputeHash hashes the document and then saves it to f.Hash.
func (f *File) ComputeHash() error {
	hasher := sha1.New()
	source, err := f.Reader()
	if err != nil {
		return err
	}
	defer source.Close()
	if _, err := io.Copy(hasher, io.LimitReader(source, MaxFileSize)); err != nil {
		return err
	}
	f.Hash = hex.EncodeToString(hasher.Sum(nil))
	return nil
}

// ComputeScore computes the rank for f and stores it f.Score.
func (f *File) ComputeScore(db *Database) float64 {
	path := strings.ToLower(f.Source)
	var score int
	for _, r := range db.CoursesNoFiles() {
		if util.RegexpMatch(r, path) {
			score++
		}
	}
	for s, rs := range FileNameScoreRegexes {
		for _, r := range rs {
			if util.RegexpMatch(r, path) {
				score += s
			}
		}
	}
	f.Score = float64(score)
	return f.Score
}

// FileSlice attaches the methods of sort.Interface to []*File, sorting in increasing order.
type FileSlice []*File

func (p FileSlice) Len() int           { return len(p) }
func (p FileSlice) Less(i, j int) bool { return p[i].Score >= p[j].Score }
func (p FileSlice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

// FileByName attaches the methods of sort.Interface to []*File, sorting in
// increasing order by name.
type FileByName []*File

func (p FileByName) Len() int { return len(p) }
func (p FileByName) Less(i, j int) bool {
	return strings.ToLower(p[i].Name) < strings.ToLower(p[j].Name)
}
func (p FileByName) Swap(i, j int) { p[i], p[j] = p[j], p[i] }
