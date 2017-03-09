package ml

import (
	"reflect"
	"testing"
	"time"

	"github.com/jbrukh/bayesian"
	"github.com/ubccsss/exams/examdb"
)

func TestURLToWords(t *testing.T) {
	cases := []struct {
		uri  string
		want []string
	}{
		{
			"https://web.archive.org/web/2222/http://www.ugrad.cs.ubc.ca:80/~cs344/current-term/resources/oldexams/cpsc344-2007w1-midterm-sample-soln.pdf",
			[]string{"https", "web", "archive", "org", "web", "2222", "http", "www", "ugrad", "cs", "ubc", "ca", "80", "cs344", "current", "term", "resources", "oldexams", "cpsc344", "2007w1", "midterm", "sample", "soln", "pdf"},
		},
		{
			"/a?b=c",
			[]string{"a", "b", "c"},
		},
		{
			" a \n b:  [(c,)] \n",
			[]string{"a", "b", "c"},
		},
		{"February 27th , 2003", []string{"february", "27th", "2003"}},
	}
	for i, c := range cases {
		out := urlToWords(c.uri)
		if !reflect.DeepEqual(out, c.want) {
			t.Errorf("%d. urlToWords(%q) = %+v; not %+v", i, c.uri, out, c.want)
		}
	}
}

func TestSplitDatesOut(t *testing.T) {
	cases := []struct {
		words []string
		want  []string
	}{
		{
			[]string{"pear2010", "foo", "cow2016w1"},
			[]string{"2010", "pear", "2016", "cow", "w1"},
		},
	}
	for i, c := range cases {
		out := splitDatesOut(c.words)
		if !reflect.DeepEqual(out, c.want) {
			t.Errorf("%d. splitDatesOut(%q) = %#v; not %#v", i, c.words, out, c.want)
		}
	}
}

func TestClassLabelExtractor(t *testing.T) {
	cases := []struct {
		name      string
		want      bayesian.Class
		extractor func(string) bayesian.Class
	}{
		{"Midterm 1 (Solution)", Solution, solutionClassFromLabel},
		{"Midterm 1", Blank, solutionClassFromLabel},
		{"Midterm 1", Midterm1, typeClassFromLabel},
		{"Sample Midterm 1", Sample, sampleClassFromLabel},
		{"Midterm 1", Real, sampleClassFromLabel},
	}
	for i, c := range cases {
		out := c.extractor(c.name)
		if !reflect.DeepEqual(out, c.want) {
			t.Errorf("%d. extractor(%q) = %+v; not %+v", i, c.name, out, c.want)
		}
	}
}

func TestGenerateNGrams(t *testing.T) {
	cases := []struct {
		words []string
		n     int
		want  []string
	}{
		{
			[]string{"foo", "cow", "duck"},
			1,
			[]string{"foo", "cow", "duck"},
		},
		{
			[]string{"foo", "cow", "duck"},
			2,
			[]string{"foo cow", "cow duck"},
		},
		{
			[]string{"foo", "cow", "duck"},
			3,
			[]string{"foo cow duck"},
		},
	}
	for i, c := range cases {
		out := generateNGrams(c.words, c.n)
		if !reflect.DeepEqual(out, c.want) {
			t.Errorf("%d. generateNGrams(%q, %d) = %#v; not %#v", i, c.words, c.n, out, c.want)
		}
	}
}

func TestExtractYearFromWords(t *testing.T) {
	cases := []struct {
		words string
		want  int
	}{
		{"", 0},
		{"2016", 2016},
		{"2015 2016", 2016},
		{"2015 2015 2016", 2015},
		{"2015 2016 January 5 2016", 2015},
		{"January 2016", 2015},
		{"April 2016", 2015},
		{"May 2016", 2016},
		{"3009 2499", 0},
		{"80 80 November 2016", 2016},
		{"https://web.archive.org/web/2222/http://www.ugrad.cs.ubc.ca/~cs414/vprev/97-t2/414mt2-key.pdf potential/414mt2-key-3.pdf CPSC 414 97-98(T2) Midterm Exam #2", 1997},
		{"http://www.ugrad.cs.ubc.ca/~cs418/2016-2/exams/midterm/a2015-2.pdf potential/a2015-2.pdf February 10, 2015 February 10, 2015", 2014},

		// st, nd, rd, th
		{"80 December 1st 2016", 2016},
		{"80 December 2nd 2016", 2016},
		{"80 December 3rd 2016", 2016},
		{"80 December 12th 2016", 2016},
		{"80 December 12th 2016", 2016},
	}
	for i, c := range cases {
		words := urlToWords(c.words)
		out, _ := ExtractYearFromWords(words)
		if out != c.want {
			t.Errorf("%d. ExtractYearFromWords(%+v) = %+v; not %+v", i, words, out, c.want)
		}
	}
}

func TestConvertDateToYearTerm(t *testing.T) {
	cases := []struct {
		date string
		year int
		term string
	}{
		{"2016 January", 2015, examdb.TermW2},
		{"2016 February", 2015, examdb.TermW2},
		{"2016 March", 2015, examdb.TermW2},
		{"2016 April", 2015, examdb.TermW2},
		{"2016 May", 2016, examdb.TermS},
		{"2016 June", 2016, examdb.TermS},
		{"2016 July", 2016, examdb.TermS},
		{"2016 August", 2016, examdb.TermS},
		{"2016 September", 2016, examdb.TermW1},
		{"2016 October", 2016, examdb.TermW1},
		{"2016 November", 2016, examdb.TermW1},
		{"2016 December", 2016, examdb.TermW1},
	}
	for i, c := range cases {
		date, err := time.Parse("2006 January", c.date)
		if err != nil {
			t.Fatal(err)
		}
		year, term := ConvertDateToYearTerm(date)
		if year != c.year || term != c.term {
			t.Errorf("%d. ExtractYearFromWords(%+v) = %+v, %+v; not %+v, %+v", i, date, year, term, c.year, c.term)
		}
	}
}
