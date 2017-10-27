package main

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/goji/httpauth"
	"github.com/ubccsss/exams/config"
	"github.com/ubccsss/exams/db"
	"github.com/ubccsss/exams/exambot"
	"github.com/ubccsss/exams/examdb"
	"github.com/ubccsss/exams/generators"
	backblaze "gopkg.in/kothar/go-backblaze.v0"
)

var (
	templates = generators.Templates

	oldDB     examdb.Database
	generator *generators.Generator
)

func loadDatabase() error {
	raw, err := ioutil.ReadFile(config.DBFile)
	if err != nil {
		return err
	}

	oldDB.Mu.Lock()
	err = json.Unmarshal(raw, &oldDB)
	oldDB.Mu.Unlock()

	if err != nil {
		return err
	}
	return nil
}

var whitespaceRegexp = regexp.MustCompile("  +")

func removeDuplicateWhitespace(str string) string {
	return whitespaceRegexp.ReplaceAllString(strings.TrimSpace(str), " ")
}

var nameToTermMapping = map[string]string{
	"(Term 1)": "W1",
	"(Term 2)": "W2",
	"(Summer)": "S",
}

var nameReplacements = map[string]string{
	"Practice Midterm":  "Sample Midterm",
	"Practice Final":    "Sample Final",
	"Final Sample":      "Sample Final",
	"Midterm 1 Sample":  "Sample Midterm 1",
	"Midterm Sample":    "Sample Midterm",
	"Midterm 2 Sample":  "Sample Midterm 2",
	"Midterm I Sample":  "Sample Midterm 1",
	"Midterm II Sample": "Sample Midterm 2",
	"Practice Quiz":     "Sample Quiz",
}

var (
	user      = flag.String("user", "admin", "")
	pass      = flag.String("pass", "", "")
	bind      = flag.String("bind", ":8080", "")
	migrate   = flag.Bool("migrate", false, "whether to migrate the database")
	dbType    = flag.String("dbtype", "postgres", "the database connection type")
	dbParams  = flag.String("db", "host=localhost user=examdb dbname=examdb sslmode=disable password=examdb", "the database connection string")
	b2Account = flag.String("b2account", "", "the backblaze account ID")
	b2Key     = flag.String("b2key", "", "the backblaze application key")
	b2Bucket  = flag.String("b2bucket", "examdb", "the backblaze bucket name")
)

func (s *server) serveSite() error {
	/*
		if err := ml.LoadOrTrainClassifier(&db, config.ClassifierDir); err != nil {
			log.Printf("Failed to load classifier. Classification tasks will not work.: %s", err)
		}
	*/

	if len(*pass) > 0 {
		secureMux := s.adminRoutes()
		http.Handle("/admin/", httpauth.SimpleBasicAuth(*user, *pass)(secureMux))
	} else {
		log.Println("No admin password set, interface disabled.")
	}

	http.HandleFunc("/upload", s.handleFileUpload)
	http.Handle("/", http.FileServer(http.Dir("static")))

	log.Printf("Listening on %s...", *bind)
	return http.ListenAndServe(*bind, nil)
}

type server struct {
	db *db.DB
}

func main() {
	log.SetOutput(os.Stderr)
	log.SetFlags(log.Flags() | log.Lshortfile)

	flag.Parse()

	var err error
	s := server{}
	s.db, err = db.Open(*dbType, *dbParams, *migrate)
	if err != nil {
		log.Fatal("failed to connect to db: %+v", err)
	}

	if err := loadDatabase(); err != nil {
		log.Printf("tried to load database: %s", err)
	}

	b2, err := backblaze.NewB2(backblaze.Credentials{
		AccountID:      *b2Account,
		ApplicationKey: *b2Key,
	})
	if err != nil {
		log.Fatal(err)
	}
	bucket, err := b2.Bucket(*b2Bucket)
	if err != nil {
		log.Fatal(err)
	}

	generator, err = generators.MakeGenerator(&oldDB, config.ExamsDir)
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		if err := exambot.Run(s.db, bucket); err != nil {
			log.Fatalf("exambot error: %+v", err)
		}
	}()

	if err := s.serveSite(); err != nil {
		log.Fatalf("%+v", err)
	}
}
