package main

import (
	"log"
	"path"
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

const (
	WinterTerm1 bayesian.Class = "W1"
	WinterTerm2 bayesian.Class = "W2"
	SummerTerm  bayesian.Class = "S"
)

// The class orders.
var (
	TypeClasses     = []bayesian.Class{Final, Midterm1, Midterm2, Midterm}
	SampleClasses   = []bayesian.Class{Sample, Real}
	SolutionClasses = []bayesian.Class{Solution, Blank}
	TermClasses     = []bayesian.Class{WinterTerm1, WinterTerm2, SummerTerm}
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

func termClassFromTerm(term string) bayesian.Class {
	term = strings.ToLower(term)
	for _, t := range TermClasses {
		if strings.Contains(term, strings.ToLower(string(t))) {
			return t
		}
	}
	return ""
}

// DocumentClassifier can classify a document into type, sample and solution
// classes.
type DocumentClassifier struct {
	TypeClassifier     *bayesian.Classifier
	SampleClassifier   *bayesian.Classifier
	SolutionClassifier *bayesian.Classifier
	TermClassifier     *bayesian.Classifier
}

// MakeDocumentClassifier trains a document classifier with all files in the DB.
func MakeDocumentClassifier() *DocumentClassifier {
	d := &DocumentClassifier{
		TypeClassifier:     bayesian.NewClassifier(TypeClasses...),
		SampleClassifier:   bayesian.NewClassifier(SampleClasses...),
		SolutionClassifier: bayesian.NewClassifier(SolutionClasses...),
		TermClassifier:     bayesian.NewClassifier(TermClasses...),
	}

	return d
}

const (
	TypeClassifierFile     = "type.classifier"
	SampleClassifierFile   = "sample.classifier"
	SolutionClassifierFile = "solution.classifier"
	TermClassifierFile     = "term.classifier"
)

// Load loads a classifier from a directory.
func (d *DocumentClassifier) Load(dir string) error {
	var err error
	d.TypeClassifier, err = bayesian.NewClassifierFromFile(path.Join(dir, TypeClassifierFile))
	if err != nil {
		return err
	}
	d.SampleClassifier, err = bayesian.NewClassifierFromFile(path.Join(dir, SampleClassifierFile))
	if err != nil {
		return err
	}
	d.SolutionClassifier, err = bayesian.NewClassifierFromFile(path.Join(dir, SolutionClassifierFile))
	if err != nil {
		return err
	}
	d.TermClassifier, err = bayesian.NewClassifierFromFile(path.Join(dir, TermClassifierFile))
	if err != nil {
		return err
	}
	return nil
}

// Save saves a classifier to a directory.
func (d DocumentClassifier) Save(dir string) error {
	if err := d.TypeClassifier.WriteToFile(path.Join(dir, TypeClassifierFile)); err != nil {
		return err
	}
	if err := d.SampleClassifier.WriteToFile(path.Join(dir, SampleClassifierFile)); err != nil {
		return err
	}
	if err := d.SolutionClassifier.WriteToFile(path.Join(dir, SolutionClassifierFile)); err != nil {
		return err
	}
	if err := d.TermClassifier.WriteToFile(path.Join(dir, TermClassifierFile)); err != nil {
		return err
	}
	return nil
}

// Train trains a classifier from the database.
func (d DocumentClassifier) Train() error {
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
					return err
				}

				if typeClass := typeClassFromLabel(f.Name); typeClass != "" {
					d.TypeClassifier.Learn(words, typeClass)
				}
				sampleClass := sampleClassFromLabel(f.Name)
				d.SampleClassifier.Learn(words, sampleClass)
				solutionClass := solutionClassFromLabel(f.Name)
				d.SolutionClassifier.Learn(words, solutionClass)
				if termClass := termClassFromTerm(f.Term); termClass != "" {
					d.TermClassifier.Learn(words, termClass)
				}

				documents++
				if documents%100 == 0 {
					log.Printf("... trained on %d", documents)
				}
			}
		}
	}

	log.Printf("Finished training document classifier! %d documents.", documents)
	if err := d.Test(); err != nil {
		return err
	}
	return nil
}

// Classify returns the most likely labels for each function.
func (d DocumentClassifier) Classify(f *File) (bayesian.Class, bayesian.Class, bayesian.Class, bayesian.Class, error) {
	words, err := fileToWordBag(f)
	if err != nil {
		return "", "", "", "", err
	}
	_, typeInx, _ := d.TypeClassifier.LogScores(words)
	_, sampleInx, _ := d.SampleClassifier.LogScores(words)
	_, solutionInx, _ := d.SolutionClassifier.LogScores(words)
	_, termInx, _ := d.TermClassifier.LogScores(words)
	return TypeClasses[typeInx], SampleClasses[sampleInx], SolutionClasses[solutionInx], TermClasses[termInx], nil
}

// Test computes the test error and prints it.
func (d DocumentClassifier) Test() error {
	log.Println("Testing document classifier...")

	documents := 0
	var typeRight, typeTotal, sampleRight, sampleTotal, solutionRight, solutionTotal, termRight, termTotal int
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
				if termClass := termClassFromTerm(f.Term); termClass != "" {
					_, inx, _ := d.TermClassifier.LogScores(words)
					termTotal++
					if TypeClasses[inx] == termClass {
						termRight++
					}
				}

				documents++
				if documents%100 == 0 {
					log.Printf("... tested on %d", documents)
				}
			}
		}
	}

	log.Printf(
		"Errors: Type %d/%d = %f, Sample %d/%d = %f, Solution %d/%d = %f, Term %d/%d = %f",
		typeRight, typeTotal, float64(typeRight)/float64(typeTotal),
		sampleRight, sampleTotal, float64(sampleRight)/float64(sampleTotal),
		solutionRight, solutionTotal, float64(solutionRight)/float64(solutionTotal),
		termRight, termTotal, float64(termRight)/float64(termTotal),
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
