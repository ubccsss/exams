package examdb

import (
	"reflect"
	"testing"

	"github.com/ubccsss/exams/config"
)

func TestCourseDepartment(t *testing.T) {
	cases := []struct {
		c    Course
		want string
	}{
		{Course{}, ""},
		{Course{Code: "wat"}, ""},
		{Course{Code: "CPSC 100"}, config.ComputerScience},
		{Course{Code: "cs100"}, config.ComputerScience},
		{Course{Code: "MATH 100"}, config.Math},
	}

	for i, c := range cases {
		out := c.c.Department()
		if out != c.want {
			t.Errorf("%d. %+v.Department() = %q; not %q", i, c.c, out, c.want)
		}
	}
}

func TestCourseAlternateIDs(t *testing.T) {
	cases := []struct {
		c    Course
		want []string
	}{
		{Course{}, nil},
		{Course{Code: "wat"}, nil},
		{Course{Code: "CPSC 101"}, []string{"cpsc 101", "cpsc101", "cpsc-101", "101"}},
		{Course{Code: "cs120"}, []string{"cs120", "cpsc120", "cpsc-120", "120", "cpsc 120"}},
		{Course{Code: "MATH 649D"}, []string{"math 649d", "math649", "math-649", "math 649"}},
	}

	for i, c := range cases {
		out := c.c.AlternateIDs()
		if !reflect.DeepEqual(out, c.want) {
			t.Errorf("%d. %+v.AlternateIDs() = %q; not %q", i, c.c, out, c.want)
		}
	}
}

func TestCourseNumber(t *testing.T) {
	cases := []struct {
		c    Course
		want int
	}{
		{Course{}, -1},
		{Course{Code: "wat"}, -1},
		{Course{Code: "CPSC 100"}, 100},
		{Course{Code: "CPSC 101"}, 101},
		{Course{Code: "cs120"}, 120},
		{Course{Code: "MATH 120"}, 120},
		{Course{Code: "MATH 649D"}, 649},
		{Course{Code: "MATH 6449D"}, 6449},
	}

	for i, c := range cases {
		out := c.c.Number()
		if out != c.want {
			t.Errorf("%d. %+v.Number() = %q; not %q", i, c.c, out, c.want)
		}
	}
}

func TestCourseYearLevel(t *testing.T) {
	cases := []struct {
		c    Course
		want string
	}{
		{Course{}, ""},
		{Course{Code: "wat"}, ""},
		{Course{Code: "CPSC 100"}, "CPSC 100"},
		{Course{Code: "CPSC 101"}, "CPSC 100"},
		{Course{Code: "cs120"}, "CPSC 100"},
		{Course{Code: "MATH 120"}, "MATH 100"},
		{Course{Code: "MATH 649D"}, "MATH 600"},
		{Course{Code: "MATH 6449D"}, "MATH 6400"},
	}

	for i, c := range cases {
		out := c.c.YearLevel()
		if out != c.want {
			t.Errorf("%d. %+v.YearLevel() = %q; not %q", i, c.c, out, c.want)
		}
	}
}
