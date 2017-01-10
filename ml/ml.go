package ml

import (
	"log"
	"math/rand"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/bbalet/stopwords"
	"github.com/d4l3k/exams/examdb"
	"github.com/d4l3k/exams/util"
	"github.com/jbrukh/bayesian"
	"github.com/sajari/docconv"
)

var yearRegex = regexp.MustCompile("(20|19)\\d{2}")

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
func (d DocumentClassifier) Train(db *examdb.Database) error {
	log.Println("Training document classifier...")
	documents := 0

	var files []*examdb.File

	for _, c := range db.Courses {
		for _, y := range c.Years {
			for _, f := range y.Files {
				if f.NotAnExam {
					continue
				}
				/*
					var found bool
					for _, l := range examdb.ExamLabels {
						if l == f.Name {
							found = true
							break
						}
					}
					if !found {
						continue
					}
				*/

				files = append(files, f)
			}
		}
	}

	for i := range files {
		j := rand.Intn(len(files))
		files[i], files[j] = files[j], files[i]
	}

	numTest := len(files) * 9 / 10
	trainFiles := files[:numTest]
	testFiles := files[numTest:]

	for _, f := range trainFiles {
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

	/*
		for _, c := range []*bayesian.Classifier{d.TypeClassifier, d.SolutionClassifier, d.SampleClassifier, d.TermClassifier} {
			c.ConvertTermsFreqToTfIdf()
		}
	*/

	log.Printf("Finished training document classifier! %d documents.", documents)
	if err := d.Test(trainFiles, testFiles, db); err != nil {
		return err
	}
	return nil
}

// Classify returns the most likely labels for each function.
func (d DocumentClassifier) Classify(f *examdb.File) (bayesian.Class, bayesian.Class, bayesian.Class, bayesian.Class, error) {
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
func (d DocumentClassifier) Test(trainFiles, testFiles []*examdb.File, db *examdb.Database) error {
	if err := d.runTest("TRAINING", trainFiles, db); err != nil {
		return err
	}
	if err := d.runTest("TEST", testFiles, db); err != nil {
		return err
	}
	return nil
}

func (d DocumentClassifier) runTest(label string, files []*examdb.File, db *examdb.Database) error {
	log.Printf("[%s] Testing document classifier...", label)

	documents := 0
	var typeRight, typeTotal, sampleRight, sampleTotal, solutionRight, solutionTotal, termRight, termTotal int
	for _, f := range files {
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
			if TermClasses[inx] == termClass {
				termRight++
			}
		}

		documents++
		if documents%100 == 0 {
			log.Printf("... tested on %d", documents)
		}
	}

	log.Printf(
		"[%s] Errors: Type %d/%d = %f, Sample %d/%d = %f, Solution %d/%d = %f, Term %d/%d = %f",
		label,
		typeRight, typeTotal, float64(typeRight)/float64(typeTotal),
		sampleRight, sampleTotal, float64(sampleRight)/float64(sampleTotal),
		solutionRight, solutionTotal, float64(solutionRight)/float64(solutionTotal),
		termRight, termTotal, float64(termRight)/float64(termTotal),
	)

	return nil
}

func fileToWordBag(f *examdb.File) ([]string, error) {
	in, err := f.Reader()
	if err != nil {
		return nil, err
	}
	defer in.Close()
	txt, _, err := docconv.ConvertPDF(in)
	if err != nil {
		return nil, err
	}
	txt = strings.ToLower(txt)
	txt = stopwords.CleanString(txt, "en", false)
	words := urlToWords(txt)
	words = append(words, urlToWords(strings.ToLower(f.Source))...)
	words = append(words, urlToWords(strings.ToLower(path.Base(f.Path)))...)

	// Add 2-grams
	twograms := generateNGrams(words, 2)

	// Split years out.
	years := splitDatesOut(words)

	words = append(words, twograms...)
	words = append(words, years...)

	return words, nil
}

func urlToWords(uri string) []string {
	if len(uri) == 0 {
		return nil
	}
	return strings.FieldsFunc(uri, func(r rune) bool {
		switch r {
		case '\t', '~', ':', ' ', '-', '.', '/', '\\', '=', '?', '\n', '(', ',', ')', '[', ']', '{', '}', '_':
			return true
		}
		return false
	})
}

func splitDatesOut(words []string) []string {
	var additional []string
	for _, word := range words {
		match := util.YearRegexp.FindString(word)
		if len(match) > 0 && len(match) != len(word) {
			additional = append(additional, match)
			rest := strings.Split(word, match)
			for _, w := range rest {
				if len(w) > 0 {
					additional = append(additional, w)
				}
			}
		}
	}
	return additional
}

func generateNGrams(words []string, n int) []string {
	var out []string
	for i := range words {
		if i > len(words)-n {
			break
		}
		out = append(out, strings.Join(words[i:i+n], " "))
	}
	return out
}

// Classifier is the default classifier set by LoadOrTrainClassifier.
var Classifier *DocumentClassifier

// LoadOrTrainClassifier loads or trains the classifier.
func LoadOrTrainClassifier(db *examdb.Database, classifierDir string) error {
	if Classifier != nil {
		return nil
	}
	log.Println("Loading classifier...")
	Classifier = MakeDocumentClassifier()
	if err := Classifier.Load(classifierDir); err != nil {
		log.Printf("Failed to load classifier: %s", err)
		if err := RetrainClassifier(db, classifierDir); err != nil {
			return err
		}
	}
	return nil
}

// RetrainClassifier retrains classifier from db and saves it to disk.
func RetrainClassifier(db *examdb.Database, classifierDir string) error {
	c := MakeDocumentClassifier()
	if err := c.Train(db); err != nil {
		return err
	}
	if err := os.MkdirAll(classifierDir, 0755); err != nil {
		return err
	}
	if err := c.Save(classifierDir); err != nil {
		return err
	}
	Classifier = c
	return nil
}

// ExtractCourse returns the predicted courseID from the file source.
func ExtractCourse(db *examdb.Database, f *examdb.File) string {
	lowerPath := strings.ToLower(f.Source)
	var bestMatch string
	var bestMatchScore int
	for _, c := range db.Courses {
		for _, id := range c.AlternateIDs() {
			if !strings.Contains(lowerPath, id) {
				continue
			}
			score := len(id)
			if score > bestMatchScore {
				bestMatch = c.Code
				bestMatchScore = score
			}
		}
	}
	return bestMatch
}
