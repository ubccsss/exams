package examdb

import (
	"reflect"
	"testing"
)

func TestIncrementFileName(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"foo.pdf", "foo-1.pdf"},
		{"foo-1.pdf", "foo-2.pdf"},
		{"foo-duck.cow.pdf", "foo-duck-1.cow.pdf"},
		{"foo-duck-1.cow.pdf", "foo-duck-2.cow.pdf"},
	}
	for i, c := range cases {
		out := incrementFileName(c.name)
		if !reflect.DeepEqual(out, c.want) {
			t.Errorf("%d. incrementFileName(%q) = %+v; not %+v", i, c.name, out, c.want)
		}
	}
}
