package db

import (
	"path"
	"strings"
	"time"

	"github.com/ubccsss/exams/examdb"
)

type File struct {
	Hash string `gorm:"primary_key"`

	FileName    string
	SourceURL   string `gorm:"index"`
	LastFetched time.Time
	StatusCode  int

	URL string
	// URLTitle is the title of the URL that lead to this file.
	URLTitle string

	Title       string
	ContentType string
	Text        string
	OCRText     string

	NoFollow bool
	Links    LinkArray `gorm:"type:text"`
	ToFetch  []ToFetch `gorm:"ForeignKey:Source"`

	RefererHash string
	Refered     []File `gorm:"ForeignKey:RefererHash"`

	Label         string
	Term          string
	Sample        bool
	Solution      bool
	NotAnExam     bool
	CourseFaculty string
	CourseCode    string
	Year          int

	HandClassified bool

	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}

func (f File) DetectedName() string {
	parts := []string{f.Label}
	if f.Sample {
		parts = append(parts, "Sample")
	}
	if f.Solution {
		parts = append(parts, "Solution")
	}
	return strings.Join(parts, " ")
}

func (db *DB) SaveFile(f *File) error {
	if err := db.DB.Where(File{Hash: f.Hash}).Assign(*f).FirstOrCreate(f).Error; err != nil {
		return err
	}
	return nil
}

// FileCount returns the file stats for the database.
func (db *DB) FileCount() examdb.FileCount {
	var c examdb.FileCount
	// TODO: implement
	return c
}

func (db *DB) TotalFileCount() (int, error) {
	var count int
	if err := db.DB.Model(File{}).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (db *DB) File(hash string) (File, error) {
	var f File
	if err := db.DB.Preload("Course").First(&f, hash).Error; err != nil {
		return File{}, err
	}
	return f, nil
}

func (db *DB) Files(filter File) ([]File, error) {
	var f []File
	if err := db.DB.Where(filter).Find(&f).Error; err != nil {
		return nil, err
	}
	return f, nil
}

func (db *DB) NotAnExamFiles() ([]File, error) {
	return db.Files(File{NotAnExam: true})
}

// FileByTerm attaches the methods of sort.Interface to []*File, sorting in
// increasing order by name.
type FileByTerm []File

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
		c := strings.ToLower(p[i].DetectedName())
		d := strings.ToLower(p[j].DetectedName())
		if c == d {
			e := strings.ToLower(path.Base(p[i].SourceURL))
			f := strings.ToLower(path.Base(p[j].SourceURL))
			return e < f
		}
		return c < d
	}
	return a < b
}
func (p FileByTerm) Swap(i, j int) { p[i], p[j] = p[j], p[i] }
