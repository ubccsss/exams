package db

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/pkg/errors"
)

type Course struct {
	Faculty string `gorm:"primary_key"`
	Code    string `gorm:"primary_key"`

	Desc string

	Files []File `gorm:"ForeignKey:CourseFaculty,CourseCode"`

	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}

func (c Course) Title() string {
	if (c.Faculty) == "" {
		return ""
	}
	return fmt.Sprintf("%s %s", c.Faculty, c.Code)
}

var courseMap = map[string]string{
	"CS": "CPSC",
}

var courseRegexp = regexp.MustCompile(`(\w{2,4})\s*(\d{3}\w?)`)

// GetCourse parses "CPSC 103" -> "CPSC", "103"
func GetCourse(course string) (string, string, error) {
	matches := courseRegexp.FindStringSubmatch(course)
	if len(matches) != 3 {
		return "", "", errors.Errorf("expected match length of 3 for %q: got %+v", course, matches)
	}
	faculty := strings.ToUpper(matches[1])
	mapped, ok := courseMap[faculty]
	if ok {
		faculty = mapped
	}
	code := strings.ToUpper(matches[2])
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
