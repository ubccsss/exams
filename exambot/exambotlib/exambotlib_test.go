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
			"https://example.com/duck/foo",
			[]string{"https://example.com/duck/foo", "https://example.com/duck/", "https://example.com/"},
		},
		{
			"https://example.com/duck/",
			[]string{"https://example.com/duck/", "https://example.com/"},
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
