package ml

import (
	"reflect"
	"strings"
	"testing"

	"github.com/jbrukh/bayesian"
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

func TestExtractYearSmart(t *testing.T) {
	cases := []struct {
		words string
		want  int
	}{
		{"", 0},
		{"2016", 2016},
		{"2015 2016", 2016},
		{"2015 2015 2016", 2015},
		{"2015 2016 January 5 2016", 2015},
	}
	for i, c := range cases {
		words := strings.Split(c.words, " ")
		out := ExtractYearSmart(words)
		if out != c.want {
			t.Errorf("%d. ExtractYearSmart(%+v) = %+v; not %+v", i, words, out, c.want)
		}
	}
}
