package examdb

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/ubccsss/exams/config"
	"github.com/ubccsss/exams/util"
)

// Term labels
const (
	TermW1      = "W1"
	TermW2      = "W2"
	TermS       = "S"
	TermUnknown = "unknown"
)

var (
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

	// ExamTerms are all the possible terms that a file can fall under.
	ExamTerms = []string{TermW1, TermW2, TermS, TermUnknown}

	// FileNameScoreRegexes are a list of regexps and values that can be used to
	// rank files based on how likely they are an exam.
	FileNameScoreRegexes = map[int][]string{
		1:  []string{"final", "exam", "midterm", "sample", "mt", "(cs|cpsc)\\d{3}", "(20|19)\\d{2}"},
		-1: []string{"report", "presentation", "thesis", "slide"},
	}
)

// File is a single exam file typically a PDF.
type File struct {
	Name           string    `json:",omitempty"`
	Path           string    `json:",omitempty"`
	Source         string    `json:",omitempty"`
	Hash           string    `json:",omitempty"`
	Score          float64   `json:",omitempty"`
	Term           string    `json:",omitempty"`
	NotAnExam      bool      `json:",omitempty"`
	Course         string    `json:",omitempty"`
	Year           int       `json:",omitempty"`
	HandClassified bool      `json:",omitempty"`
	Updated        time.Time `json:",omitempty"`

	LastResponseCode int `json:",omitempty"`

	// Inferred is the results that are inferred via ML.
	Inferred *File `json:",omitempty"`
}

// PathOnDisk returns the path to the file on disk.
func (f File) PathOnDisk() string {
	// Only for tests.
	if filepath.IsAbs(f.Path) && strings.HasPrefix(f.Path, os.TempDir()) {
		return f.Path
	}
	return path.Join(config.ExamsDir, f.Path)
}

// IsPotential returns whether the file has been processed yet.
func (f File) IsPotential() bool {
	return !(f.NotAnExam || f.HandClassified)
}

// Reader opens the file either over HTTP or from disk and returns an
// io.ReadCloser which needs to be closed by the caller.
func (f *File) Reader() (io.ReadCloser, error) {
	var source io.ReadCloser
	if len(f.Path) > 0 {
		var err error
		source, err = os.Open(f.PathOnDisk())
		if err != nil {
			return nil, err
		}
	} else if len(f.Source) > 0 {
		req, err := http.Get(f.Source)
		if err != nil {
			return nil, err
		}
		f.LastResponseCode = req.StatusCode
		if req.StatusCode != http.StatusOK {
			req.Body.Close()
			return nil, errors.Errorf("expected http.Get(%q).StatusCode to be 200; got %d", f.Source, req.StatusCode)
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
	if _, err := io.Copy(hasher, io.LimitReader(source, config.MaxFileSize)); err != nil {
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

// IdealDir returns the directory the file should be in.
func (f File) IdealDir() string {
	if f.NotAnExam {
		return "notanexam"
	}
	if f.IsPotential() || len(f.Course) == 0 {
		return "potential"
	}
	return fmt.Sprintf("%s/%d", f.Course, f.Year)
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
	a := strings.ToLower(p[i].Name)
	b := strings.ToLower(p[j].Name)
	if a == b {
		return strings.ToLower(p[i].Path) < strings.ToLower(p[j].Path)
	}
	return a < b
}
func (p FileByName) Swap(i, j int) { p[i], p[j] = p[j], p[i] }

// FileByTerm attaches the methods of sort.Interface to []*File, sorting in
// increasing order by name.
type FileByTerm []*File

func (p FileByTerm) Len() int { return len(p) }

var termOrder = map[string]int{
	"W1": -3,
	"W2": -2,
	"S":  -1,
}

func (p FileByTerm) Less(i, j int) bool {
	a := termOrder[p[i].Term]
	b := termOrder[p[j].Term]
	if a == b {
		c := strings.ToLower(p[i].Name)
		d := strings.ToLower(p[j].Name)
		if c == d {
			e := strings.ToLower(p[i].Path)
			f := strings.ToLower(p[j].Path)
			return e < f
		}
		return c < d
	}
	return a < b
}
func (p FileByTerm) Swap(i, j int) { p[i], p[j] = p[j], p[i] }

// FileByYearTermName attaches the methods of sort.Interface to []*File, sorting in
// decreasing order by year, then increasing order by term and name.
type FileByYearTermName []*File

func (p FileByYearTermName) Len() int { return len(p) }
func (p FileByYearTermName) Less(i, j int) bool {
	// Sort by year
	a := fileYear(p[i])
	b := fileYear(p[j])
	if a == b {
		// Sort by term
		at := fileTerm(p[i])
		bt := fileTerm(p[j])
		if at == bt {
			// Sort by name
			return strings.ToLower(p[i].Name) < strings.ToLower(p[j].Name)
		}
		return at < bt
	}
	return a >= b
}
func (p FileByYearTermName) Swap(i, j int) { p[i], p[j] = p[j], p[i] }

func fileYear(f *File) int {
	if f.Year > 0 {
		return f.Year
	}
	if f.Inferred != nil {
		return f.Inferred.Year
	}
	return 0
}

func fileTerm(f *File) string {
	if f.Term != "" {
		return f.Term
	}
	if f.Inferred != nil {
		return f.Inferred.Term
	}
	return ""
}
