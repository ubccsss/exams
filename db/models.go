package db

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	"github.com/pkg/errors"
)

var migrateLock sync.Mutex

// MigrateDB migrates the database models. This should only be called by the db
// and testdb packages.
func MigrateDB(db *gorm.DB) error {
	models := []interface{}{
		&File{},
		&ToFetch{},
		&Course{},
	}

	migrateLock.Lock()
	defer migrateLock.Unlock()

	for _, model := range models {
		if err := db.AutoMigrate(model).Error; err != nil {
			return errors.Wrapf(err, "failed to migrate %T", model)
		}
	}
	return nil
}

type ToFetch struct {
	URL    string `gorm:"primary_key"`
	Title  string
	Source File

	CreatedAt time.Time
}

type File struct {
	Hash string `gorm:"primary_key"`

	FileName    string
	SourceURL   string `gorm:"index"`
	LastFetched time.Time
	StatusCode  int

	URL string

	Title       string
	ContentType string
	Text        string
	OCRText     string

	Links   LinkArray
	ToFetch []ToFetch

	Referer      string
	RefererTitle string

	Label     string
	Term      string
	NotAnExam bool
	Course    Course
	Year      int

	HandClassified bool

	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}

type Course struct {
	Faculty string `gorm:"primary_key"`
	Code    int    `gorm:"primary_key"`

	Desc string

	Files []File

	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}

func (c Course) Title() string {
	if (c.Faculty) == "" {
		return ""
	}
	return fmt.Sprintf("%s %03d", c.Faculty, c.Code)
}

type Link struct {
	Title string
	URL   string
}

type LinkArray []Link

func (s LinkArray) Value() (driver.Value, error) {
	j, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	return string(j), nil
}

func (s *LinkArray) Scan(src interface{}) error {
	source, ok := src.([]byte)
	if !ok {
		s, ok := src.(string)
		if !ok {
			return errors.Errorf("Type assertion .([]byte,string) failed. Type: %T", src)
		}
		source = []byte(s)
	}

	if err := json.Unmarshal(source, s); err != nil {
		return err
	}

	return nil
}
