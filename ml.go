package main

import (
	"log"
	"strings"

	"github.com/jbrukh/bayesian"
	"github.com/sajari/docconv"
)

// Exam type
const (
	Midterm  bayesian.Class = "Midterm"
	Midterm1 bayesian.Class = "Midterm 1"
	Midterm2 bayesian.Class = "Midterm 2"
	Final    bayesian.Class = "Final"
)

// Sample or not
const (
	Sample bayesian.Class = "Sample"
	Real   bayesian.Class = ""
)

// Exam solution or not
const (
	Solution bayesian.Class = "Solution"
	Blank    bayesian.Class = ""
)

// The class orders.
var (
	TypeClasses     = []bayesian.Class{Final, Midterm1, Midterm2, Midterm}
	SampleClasses   = []bayesian.Class{Sample, Real}
	SolutionClasses = []bayesian.Class{Solution, Blank}
)

func typeClassFromLabel(label string) bayesian.Class {
	label = strings.ToLower(label)
	for _, t := range TypeClasses {
		if strings.Contains(label, strings.ToLower(string(t))) {
			return t
		}
	}
	return ""
}

func sampleClassFromLabel(label string) bayesian.Class {
	label = strings.ToLower(label)
	if strings.Contains(label, strings.ToLower(string(Sample))) {
		return Sample
	}
	return Real
}

func solutionClassFromLabel(label string) bayesian.Class {
	label = strings.ToLower(label)
	if strings.Contains(label, strings.ToLower(string(Solution))) {
		return Solution
	}
	return Blank
}

// DocumentClassifier can classify a document into type, sample and solution
// classes.
type DocumentClassifier struct {
	TypeClassifier     *bayesian.Classifier
	SampleClassifier   *bayesian.Classifier
	SolutionClassifier *bayesian.Classifier
}

// MakeDocumentClassifier trains a document classifier with all files in the DB.
func MakeDocumentClassifier() (*DocumentClassifier, error) {
	typeClassifier := bayesian.NewClassifier(TypeClasses...)
	sampleClassifier := bayesian.NewClassifier(SampleClasses...)
	solutionClassifier := bayesian.NewClassifier(SolutionClasses...)

	log.Println("Training document classifier...")
	documents := 0

	for _, c := range db.Courses {
		for _, y := range c.Years {
			for _, f := range y.Files {
				if f.NotAnExam {
					continue
				}

				words, err := fileToWordBag(f)
				if err != nil {
					return nil, err
				}

				if typeClass := typeClassFromLabel(f.Name); typeClass != "" {
					typeClassifier.Learn(words, typeClass)
				}
				sampleClass := sampleClassFromLabel(f.Name)
				sampleClassifier.Learn(words, sampleClass)
				solutionClass := solutionClassFromLabel(f.Name)
				solutionClassifier.Learn(words, solutionClass)

				documents++
				if documents%100 == 0 {
					log.Printf("... trained on %d", documents)
				}
			}
		}
	}

	log.Printf("Finished training document classifier! %d documents.", documents)

	d := &DocumentClassifier{
		TypeClassifier:     typeClassifier,
		SampleClassifier:   sampleClassifier,
		SolutionClassifier: solutionClassifier,
	}
	if err := d.Test(); err != nil {
		return nil, err
	}
	return d, nil
}

// Classify returns the most likely labels for each function.
func (d DocumentClassifier) Classify(f *File) (bayesian.Class, bayesian.Class, bayesian.Class, error) {
	words, err := fileToWordBag(f)
	if err != nil {
		return "", "", "", err
	}
	_, typeInx, _ := d.TypeClassifier.LogScores(words)
	_, sampleInx, _ := d.SampleClassifier.LogScores(words)
	_, solutionInx, _ := d.SolutionClassifier.LogScores(words)
	return TypeClasses[typeInx], SampleClasses[sampleInx], SolutionClasses[solutionInx], nil
}

func (d DocumentClassifier) Test() error {
	log.Println("Testing document classifier...")

	var typeRight, typeTotal, sampleRight, sampleTotal, solutionRight, solutionTotal int
	for _, c := range db.Courses {
		for _, y := range c.Years {
			for _, f := range y.Files {
				if f.NotAnExam {
					continue
				}

				words, err := fileToWordBag(f)
				if err != nil {
					return err
				}

				if typeClass := typeClassFromLabel(f.Name); typeClass != "" {
					_, inx, _ := d.TypeClassifier.LogScores(words)
					typeTotal++
					if TypeClasses[inx] == typeClass {
						typeRight++
					}
				}
				sampleClass := sampleClassFromLabel(f.Name)
				_, inx, _ := d.SampleClassifier.LogScores(words)
				sampleTotal++
				if SampleClasses[inx] == sampleClass {
					sampleRight++
				}
				solutionClass := solutionClassFromLabel(f.Name)
				_, inx, _ = d.SolutionClassifier.LogScores(words)
				solutionTotal++
				if SolutionClasses[inx] == solutionClass {
					solutionRight++
				}
			}
		}
	}

	log.Printf(
		"Errors: Type %d/%d = %f, Sample %d/%d = %f, Solution %d/%d = %f",
		typeRight, typeTotal, float64(typeRight)/float64(typeTotal),
		sampleRight, sampleTotal, float64(sampleRight)/float64(sampleTotal),
		solutionRight, solutionTotal, float64(solutionRight)/float64(solutionTotal),
	)

	return nil
}

func fileToWordBag(f *File) ([]string, error) {
	in, err := f.reader()
	if err != nil {
		return nil, err
	}
	defer in.Close()
	txt, _, err := docconv.ConvertPDF(in)
	if err != nil {
		return nil, err
	}
	words := strings.Split(strings.ToLower(txt), " ")
	words = append(words, urlToWords(strings.ToLower(f.Source))...)
	return words, nil
}

func urlToWords(uri string) []string {
	if len(uri) == 0 {
		return nil
	}
	return strings.FieldsFunc(uri, func(r rune) bool {
		switch r {
		case '~', ':', ' ', '-', '.', '/', '\\', '=', '?':
			return true
		}
		return false
	})
}
