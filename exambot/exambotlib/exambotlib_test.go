package exambotlib

import (
	"reflect"
	"testing"
)

func TestExpandURLToParents(t *testing.T) {
	cases := []struct {
		uri  string
		want []string
	}{
		{
			"https://example.com/",
			[]string{"https://example.com/"},
		},
		{
			"https://example.com",
			[]string{"https://example.com"},
		},
		{
			"https://example.com/duck/foo",
			[]string{"https://example.com/duck/foo", "https://example.com/duck/", "https://example.com/"},
		},
		{
			"https://example.com/duck/",
			[]string{"https://example.com/duck/", "https://example.com/"},
		},
		{
			"https://example.com/duck/?foo=10",
			[]string{"https://example.com/duck/?foo=10", "https://example.com/duck/", "https://example.com/"},
		},
		{
			"https://example.com/duck?foo=10",
			[]string{"https://example.com/duck?foo=10", "https://example.com/duck", "https://example.com/"},
		},
	}

	for i, c := range cases {
		out, err := ExpandURLToParents(c.uri)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(out, c.want) {
			t.Errorf("%d. ExpandURLToParents(%q) = %+v; not %+v", i, c.uri, out, c.want)
		}
	}
}

func TestValidSuffix(t *testing.T) {
	cases := []struct {
		uri  string
		want bool
	}{
		{
			"https://example.com/",
			true,
		},
		{
			"https://example.com",
			false,
		},
		{
			"https://example.com/duck/foo",
			true,
		},
		{
			"https://example.com/duck/foo.php",
			true,
		},
		{
			"https://example.com/duck/foo.pdf",
			false,
		},
		{
			"https://example.com/duck/?foo=10",
			true,
		},
	}

	for i, c := range cases {
		out := ValidSuffix(c.uri, []string{".php", "/"})
		if !reflect.DeepEqual(out, c.want) {
			t.Errorf("%d. ValidSuffix(%q) = %+v; not %+v", i, c.uri, out, c.want)
		}
	}
}
