package db

import (
	"log"

	"github.com/jinzhu/gorm"
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

func (db *DB) PopulateSeenVisited(seen *bloom.BloomFilter) error {
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
		seen.AddString(url)
		count += 1
	}
	log.Printf("Loaded %d ToFetchs.", count)

	return nil
}
