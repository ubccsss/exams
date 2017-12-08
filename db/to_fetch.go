package db

import (
	"bytes"
	"fmt"
	"time"

	"github.com/pkg/errors"
)

// ToFetch holds a URL that should be fetched and processed. If DeletedAt is
// set, that means we've either already processed the URL, or we don't want to.
type ToFetch struct {
	URL   string `gorm:"primary_key"`
	Title string

	// Source is the file hash of the source.
	Source string

	CreatedAt time.Time
	// DeletedAt is set if the URL has been visited.
	DeletedAt *time.Time
}

// AddToFetch adds a ToFetch object to the database or updates the existing one
// if it already exists.
func (db *DB) AddToFetch(tf ToFetch) error {
	return db.DB.Where(ToFetch{URL: tf.URL}).Assign(tf).FirstOrCreate(&tf).Error
}

// BulkAddToFetch bulk adds ToFetch entries to the database. If there's a URL
// conflict, it is not inserted.
func (db *DB) BulkAddToFetch(tfs []ToFetch) (int, error) {
	if len(tfs) == 0 {
		return 0, nil
	}

	query := `INSERT INTO to_fetches (url, title, source)
	VALUES %s
	ON CONFLICT (url) DO NOTHING`
	var params bytes.Buffer
	var args []interface{}
	for i, tf := range tfs {
		if i != 0 {
			params.WriteString(",")
		}
		params.WriteString("(?,?,?)")
		args = append(args, tf.URL, tf.Title, tf.Source)
	}

	resp := db.DB.Exec(fmt.Sprintf(query, params.String()), args...)
	return int(resp.RowsAffected), resp.Error
}

// DeleteToFetch marks a URL as deleted.
func (db *DB) DeleteToFetch(url string) error {
	if len(url) == 0 {
		return errors.New("url must not be empty")
	}
	return db.DB.Delete(ToFetch{URL: url}).Error
}

// RandomToFetch returns count ToFetch items randomly sampled from the database.
func (db *DB) RandomToFetch(count int) ([]ToFetch, error) {
	var tf []ToFetch
	if err := db.DB.
		Unscoped().
		Raw("SELECT url, title, source, created_at, deleted_at FROM to_fetches TABLESAMPLE BERNOULLI (least(100, ? * 100.0 / (SELECT greatest(count(*), 100) FROM to_fetches WHERE deleted_at is NULL))) WHERE deleted_at is NULL", count).
		Find(&tf).Error; err != nil {
		return nil, err
	}
	return tf, nil
}

// ToFetchCount returns the number of entries that still need to be fetched.
func (db *DB) ToFetchCount() (int, error) {
	var count int
	if err := db.DB.Model(ToFetch{}).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// SeenURLCount returns the total number of seen URLs in the database.
func (db *DB) SeenURLCount() (int, error) {
	var count int
	if err := db.DB.Model(ToFetch{}).Unscoped().Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}
