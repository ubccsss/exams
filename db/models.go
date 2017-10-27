package db

import (
	"database/sql/driver"
	"encoding/json"
	"sync"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	"github.com/pkg/errors"
)

var migrateLock sync.Mutex

var models = []interface{}{
	&File{},
	&ToFetch{},
	&Course{},
}

// MigrateDB migrates the database models. This should only be called by the db
// and testdb packages.
func MigrateDB(db *gorm.DB) error {

	migrateLock.Lock()
	defer migrateLock.Unlock()

	for _, model := range models {
		if err := db.AutoMigrate(model).Error; err != nil {
			return errors.Wrapf(err, "failed to migrate %T", model)
		}
	}
	return nil
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
