package main

import (
	"reflect"
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
	}
	for i, c := range cases {
		out := urlToWords(c.uri)
		if !reflect.DeepEqual(out, c.want) {
			t.Errorf("%d. urlToWords(%q) = %+v; not %+v", i, c.uri, out, c.want)
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
