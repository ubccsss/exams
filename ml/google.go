package ml

import (
	"context"
	"encoding/csv"
	"fmt"
	"log"
	"path"
	"strings"
	"sync"

	"golang.org/x/oauth2/google"
	prediction "google.golang.org/api/prediction/v1.6"

	"github.com/ubccsss/exams/examdb"

	"cloud.google.com/go/storage"
)

const (
	projectID  = "genial-airway-99405"
	bucketName = "exams"
)

type fileClassLabeler func(*examdb.File) (string, bool)

var fileClassifiers = map[string]fileClassLabeler{
	"type": func(f *examdb.File) (string, bool) {
		class := typeClassFromLabel(f.Name)
		return string(class), len(class) > 0
	},
	"solution": func(f *examdb.File) (string, bool) {
		return string(solutionClassFromLabel(f.Name)), true
	},
	"sample": func(f *examdb.File) (string, bool) {
		return string(sampleClassFromLabel(f.Name)), true
	},
	"term": func(f *examdb.File) (string, bool) {
		return string(termClassFromTerm(f.Term)), len(f.Term) > 0
	},
}

// GoogleClassifier is a ML classifier that uses Google Cloud Prediction.
type GoogleClassifier struct {
	Trainedmodels *prediction.TrainedmodelsService
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
	}, nil
}

func fileFeatures(f *examdb.File) ([]string, error) {
	source := f.Source
	if len(source) == 0 {
		source = f.Path
	}
	source = strings.Join(urlToWords(source), " ")
	wordBag, err := fileContentWords(f)
	if err != nil {
		return nil, err
	}
	return []string{source, strings.Join(wordBag, " ")}, nil
}

// Train uploads the data to GCE and trains a Google Cloud Prediction model.
func (c *GoogleClassifier) Train(db *examdb.Database) error {
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

	var files []*examdb.File

	for _, c := range db.Courses {
		for _, y := range c.Years {
			for _, f := range y.Files {
				if f.NotAnExam {
					continue
				}

				files = append(files, f)
			}
		}
	}

	count := 0
	for _, f := range files {
		features, err := fileFeatures(f)
		if err != nil {
			return err
		}
		for classifier, fun := range fileClassifiers {
			class, ok := fun(f)
			if !ok {
				continue
			}
			if err := csvWriters[classifier].Write(append([]string{class}, features...)); err != nil {
				return err
			}
		}

		count++
		if count%100 == 0 {
			log.Printf("uploaded %d...", count)
		}
	}

	for _, w := range writers {
		if err := w.Close(); err != nil {
			return err
		}
	}

	log.Printf("Done uploading. Training model ...")

	for classifier := range fileClassifiers {
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
func (c *GoogleClassifier) Classify(f *examdb.File) (map[string]string, error) {
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
	for class := range fileClassifiers {
		wg.Add(1)
		class := class
		go func() {
			defer wg.Done()
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
	wg.Wait()
	if err != nil {
		return nil, err
	}

	return m, nil
}
