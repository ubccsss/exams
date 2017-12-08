package db

import (
	"log"
	"net/http"

	"github.com/jinzhu/gorm"
	"github.com/qor/admin"
	"github.com/willf/bloom"
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

func (db *DB) PopulateSeenVisited(urls *bloom.BloomFilter, hashes *bloom.BloomFilter) error {
	{
		rows, err := db.DB.Model(ToFetch{}).Select("url").Unscoped().Rows()
		if err != nil {
			return err
		}
		defer rows.Close()

		count := 0
		for rows.Next() {
			var url string
			if err := rows.Scan(&url); err != nil {
				return err
			}
			urls.AddString(url)
			count += 1
		}
		log.Printf("Loaded %d ToFetch urls.", count)
	}

	{
		rows, err := db.DB.Model(File{}).Select("hash").Unscoped().Rows()
		if err != nil {
			return err
		}
		defer rows.Close()

		count := 0
		for rows.Next() {
			var hash string
			if err := rows.Scan(&hash); err != nil {
				return err
			}
			hashes.AddString(hash)
			count += 1
		}
		log.Printf("Loaded %d File hashes.", count)
	}

	return nil
}

func (db *DB) AdminMux(path string) *http.ServeMux {
	a := admin.New(&admin.AdminConfig{DB: db.DB})
	course := a.AddResource(&Course{})
	course.NewAttrs("Faculty", "Code", "Desc")
	course.EditAttrs("Desc")
	course.IndexAttrs("Faculty", "Code", "Desc", "CreatedAt", "UpdatedAt")

	mux := http.NewServeMux()
	a.MountTo(path, mux)
	return mux
}
