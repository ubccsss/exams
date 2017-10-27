package db

import (
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

	Links   LinkArray `gorm:"type:text"`
	ToFetch []ToFetch `gorm:"ForeignKey:Source"`

	RefererHash string
	Refered     []File `gorm:"ForeignKey:RefererHash"`

	Label         string
	Term          string
	NotAnExam     bool
	CourseFaculty string
	CourseCode    int
	Year          int

	HandClassified bool

	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
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
