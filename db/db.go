package db

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/pkg/errors"
	"github.com/ubccsss/exams/examdb"
)

type DB struct {
	DB *gorm.DB
}

// Open opens a connection to the database.
func Open(dbtype, path string, migrate bool) (*DB, error) {
	db, err := gorm.Open(dbtype, path)
	if err != nil {
		return nil, err
	}
	if migrate {
		if err := MigrateDB(db); err != nil {
			return nil, err
		}
	}
	return &DB{
		DB: db,
	}, nil
}

func (db *DB) SaveFile(f *File) error {
	if err := db.DB.Where(File{Hash: f.Hash}).Assign(*f).FirstOrCreate(&f).Error; err != nil {
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

var courseMap = map[string]string{
	"CS": "CPSC",
}
var courseRegexp = regexp.MustCompile(`(\w{2,4})\s*(\d{3})`)

// GetCourse parses "CPSC 103" -> "CPSC", 103
func GetCourse(course string) (string, int, error) {
	matches := courseRegexp.FindStringSubmatch(course)
	if len(matches) != 3 {
		return "", 0, errors.Errorf("expected match length of 3 for %q: got %+v", course, matches)
	}
	faculty := strings.ToUpper(matches[1])
	mapped, ok := courseMap[faculty]
	if ok {
		faculty = mapped
	}
	code, err := strconv.Atoi(matches[2])
	if err != nil {
		return "", 0, err
	}
	return faculty, code, nil
}

func (db *DB) Course(course string) (Course, error) {
	var c Course
	faculty, code, err := GetCourse(course)
	if err != nil {
		return Course{}, err
	}
	if err := db.DB.Where(Course{
		Faculty: faculty,
		Code:    code,
	}).First(&c).Error; err != nil {
		return Course{}, err
	}
	return c, nil
}

func (db *DB) Courses(filter Course) ([]Course, error) {
	var f []Course
	if err := db.DB.Where(filter).Find(&f).Error; err != nil {
		return nil, err
	}
	return f, nil
}

func (db *DB) DisplayCourses() ([]string, error) {
	courses, err := db.Courses(Course{Faculty: "CPSC"})
	if err != nil {
		return nil, err
	}

	var disp []string
	for _, c := range courses {
		disp = append(disp, c.Title())
	}
	return disp, nil
}
