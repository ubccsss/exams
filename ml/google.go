package ml

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"log"
	"path"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/oauth2/google"
	"golang.org/x/time/rate"
	prediction "google.golang.org/api/prediction/v1.6"

	"github.com/ubccsss/exams/examdb"
	"github.com/ubccsss/exams/workers"

	"cloud.google.com/go/storage"
)

const (
	projectID  = "genial-airway-99405"
	bucketName = "exams"
)

type fileClassLabeler func(*examdb.File) (string, bool)

const (
	IsExam    = "yes"
	IsNotExam = "no"
)

var fileClassifiers = map[string]fileClassLabeler{
	"type": func(f *examdb.File) (string, bool) {
		if f.NotAnExam {
			return "", false
		}
		class := typeClassFromLabel(f.Name)
		return string(class), len(class) > 0
	},
	"solution": func(f *examdb.File) (string, bool) {
		if f.NotAnExam {
			return "", false
		}
		return string(solutionClassFromLabel(f.Name)), true
	},
	"sample": func(f *examdb.File) (string, bool) {
		if f.NotAnExam {
			return "", false
		}
		return string(sampleClassFromLabel(f.Name)), true
	},
	"term": func(f *examdb.File) (string, bool) {
		if f.NotAnExam {
			return "", false
		}
		return string(termClassFromTerm(f.Term)), len(f.Term) > 0
	},
	"isexam": func(f *examdb.File) (string, bool) {
		if f.NotAnExam {
			return IsNotExam, true
		}
		return IsExam, true
	},
}

// GoogleClassifier is a ML classifier that uses Google Cloud Prediction.
type GoogleClassifier struct {
	Trainedmodels *prediction.TrainedmodelsService
	Limiter       *rate.Limiter
}

// MakeGoogleClassifier creates a new classifier.
func MakeGoogleClassifier() (*GoogleClassifier, error) {
	httpClient, err := google.DefaultClient(context.Background())
	if err != nil {
		return nil, err
	}
	predictionService, err := prediction.New(httpClient)
	return &GoogleClassifier{
		Trainedmodels: predictionService.Trainedmodels,

		// Google Prediction API allows for 100 requests every 100 seconds.
		// *it's been increased to 1000/100s
		Limiter: rate.NewLimiter(rate.Limit(1000/100), 1000),
	}, nil
}

func fileFeatures(f *examdb.File) ([]string, error) {
	source := f.Source
	if len(source) == 0 {
		source = f.Path
	}
	source = strings.Join(urlToWords(source), " ")
	wordBag, meta, err := fileToWordBagMeta(f)
	if err != nil {
		return nil, err
	}
	year, term := ExtractYearFromWords(wordBag)
	features := []string{source, strings.Join(wordBag, " "), strconv.Itoa(year), term}
	for _, prop := range []string{
		"Author",
		"File size",
		"Pages",
		"Page size",
		"CreationDate",
		"ModDate",
		"Title",
	} {
		featureWords := urlToWords(meta[prop])
		features = append(features, strings.Join(featureWords, " "))
	}
	return features, nil
}

// Train uploads the data to GCE and trains a Google Cloud Prediction model.
func (c *GoogleClassifier) Train(db *examdb.Database) error {
	if c == nil {
		return errors.New("classifier has not been loaded")
	}

	log.Printf("Uploading to GCE")
	ctx := context.Background()

	// Your Google Cloud Platform project ID

	// Creates a client
	client, err := storage.NewClient(ctx)
	if err != nil {
		return err
	}

	// Prepares a new bucket
	bucket := client.Bucket(bucketName)

	if _, err := bucket.Attrs(ctx); err != nil {
		return err
	}

	writers := map[string]*storage.Writer{}
	csvWriters := map[string]*csv.Writer{}
	fileNames := map[string]string{}
	for classifier := range fileClassifiers {
		fileName := fmt.Sprintf("%sclassifier.csv", classifier)
		obj := bucket.Object(fileName)
		w := obj.NewWriter(ctx)
		defer w.Close()
		writers[classifier] = w
		csvWriters[classifier] = csv.NewWriter(w)
		fileNames[classifier] = fileName
	}

	fileChan := make(chan *examdb.File)
	go func() {
		db.Mu.RLock()
		defer db.Mu.RUnlock()
		for _, f := range db.Files {
			if f.NotAnExam || f.HandClassified {
				fileChan <- f
			}
		}
		close(fileChan)
	}()

	type featuresClass struct {
		classifier string
		features   []string
	}
	featureChan := make(chan featuresClass, workers.Count)

	var wg sync.WaitGroup
	count := 0
	for i := 0; i < workers.Count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for f := range fileChan {
				classes := map[string]string{}
				for classifier, fun := range fileClassifiers {
					class, ok := fun(f)
					if !ok {
						continue
					}
					classes[classifier] = class
				}
				if len(classes) == 0 {
					continue
				}
				features, err := fileFeatures(f)
				if err != nil {
					log.Println(err)
					continue
				}
				for classifier, class := range classes {
					featureChan <- featuresClass{
						classifier: classifier,
						features:   append([]string{class}, features...),
					}
				}

				count++
				if count%100 == 0 {
					log.Printf("uploaded %d...", count)
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(featureChan)
	}()

	for f := range featureChan {
		if err := csvWriters[f.classifier].Write(f.features); err != nil {
			return err
		}
	}

	for _, w := range writers {
		if err := w.Close(); err != nil {
			return err
		}
	}

	log.Printf("Done uploading. Training model ...")

	for classifier := range fileClassifiers {
		if err := c.Limiter.Wait(ctx); err != nil {
			return err
		}
		call := c.Trainedmodels.Insert(projectID, &prediction.Insert{
			Id:                  classifierModelName(classifier),
			StorageDataLocation: path.Join("exams", fileNames[classifier]),
		})
		resp, err := call.Do()
		if err != nil {
			return err
		}
		log.Printf("%s: Status %s %s %+v", classifier, resp.TrainingStatus, resp.TrainingComplete, resp.ModelInfo)
	}

	return nil
}

func classifierModelName(classifier string) string {
	return fmt.Sprintf("%sclassifier", classifier)
}

// Classify returns the most likely labels for each function.
func (c *GoogleClassifier) Classify(f *examdb.File, lazy bool) (map[string]string, error) {
	if c == nil {
		return nil, errors.New("classifier has not been loaded")
	}

	features, err := fileFeatures(f)
	if err != nil {
		return nil, err
	}
	interfaceFeatures := make([]interface{}, len(features))
	for i, f := range features {
		interfaceFeatures[i] = f
	}
	input := prediction.Input{
		Input: &prediction.InputInput{CsvInstance: interfaceFeatures},
	}
	m := map[string]string{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	ctx := context.Background()
	classifyInner := func(class string) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err2 := c.Limiter.Wait(ctx); err2 != nil {
				err = err2
				return
			}
			call := c.Trainedmodels.Predict(projectID, classifierModelName(class), &input)
			resp, err2 := call.Do()
			if err2 != nil {
				err = err2
				return
			}
			mu.Lock()
			defer mu.Unlock()
			m[class] = resp.OutputLabel
		}()
	}
	if lazy {
		classifyInner("isexam")
		wg.Wait()
		if err != nil {
			return nil, err
		}
	}
	if m["isexam"] == IsExam || !lazy {
		for class := range fileClassifiers {
			if lazy && class == "isexam" {
				continue
			}
			classifyInner(class)
		}
		wg.Wait()
		if err != nil {
			return nil, err
		}
	}
	log.Printf("m: %+v", m)

	return m, nil
}

// ReportAccuracy creates a report of how accurate the classifier is.
func (c *GoogleClassifier) ReportAccuracy(w io.Writer) error {
	if c == nil {
		return errors.New("classifier has not been loaded")
	}

	ctx := context.Background()
	for class := range fileClassifiers {
		fmt.Fprintf(w, "%s\n", class)
		if err := c.Limiter.Wait(ctx); err != nil {
			return err
		}
		call := c.Trainedmodels.Get(projectID, classifierModelName(class))
		resp, err := call.Do()
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "  - status: %s\n", resp.TrainingStatus)
		fmt.Fprintf(w, "  - accuracy: %#v\n", resp.ModelInfo)
	}
	return nil
}
