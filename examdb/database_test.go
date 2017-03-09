package examdb

import (
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"sort"
	"sync"
	"testing"
)

var testFiles struct {
	sync.Mutex

	files []string
}

func cleanupTestFiles(t *testing.T) {
	testFiles.Lock()
	defer testFiles.Unlock()

	for _, f := range testFiles.files {
		if err := os.Remove(f); err != nil {
			t.Error(err)
		}
	}
	testFiles.files = nil
}

func testFile(t *testing.T) *File {
	tmpfile, err := ioutil.TempFile("", "testfile.txt")
	if err != nil {
		t.Fatal(err)
	}

	content := fmt.Sprintf(`%s
This is a test file for github.com/ubccsss/examdb.
It is created automatically during tests and should be removed automatically as well.
`, tmpfile.Name())

	if _, err := tmpfile.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	testFiles.Lock()
	testFiles.files = append(testFiles.files, tmpfile.Name())
	testFiles.Unlock()

	file := &File{
		Path:   tmpfile.Name(),
		Course: "test 101",
	}

	return file
}

func TestAddFile(t *testing.T) {
	defer cleanupTestFiles(t)

	db := MakeDatabase()
	a := testFile(t)
	b := testFile(t)
	c := testFile(t)

	// Add files a bunch with repeats, should dedup.
	for _, f := range []*File{a, b, c, a, b, c, a, b, c} {
		if err := db.AddFile(f); err != nil {
			t.Fatal(err)
		}
	}

	// Creates course
	{
		want := map[string]*Course{
			"test 101": {Code: "test 101"},
		}
		if !reflect.DeepEqual(db.Courses, want) {
			t.Errorf("adding a file should create the corresponding course. got %#v; want %#v", db.Courses, want)
		}
	}

	// Adds and deduplicates.
	{
		want := []*File{
			a, b, c,
		}
		out := db.Files
		sort.Sort(FileByName(want))
		sort.Sort(FileByName(out))
		if !reflect.DeepEqual(out, want) {
			t.Errorf("adding a set of files should dedup and add. got %#v; want %#v", out, want)
		}
	}
}

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

func TestDisplayCourses(t *testing.T) {
	cases := []struct {
		db   *Database
		want []string
	}{
		{
			&Database{
				Courses: map[string]*Course{},
			},
			nil,
		},
		{
			&Database{
				Courses: map[string]*Course{
					"cpsc 120": {
						Code: "cpsc 120",
					},
					"cpsc 101": {
						Code: "cpsc 101",
					},
					"math 100": {
						Code: "math 100",
					},
					"": {},
				},
			},
			[]string{"cpsc 101", "cpsc 120"},
		},
	}
	for i, c := range cases {
		out := c.db.DisplayCourses()
		if !reflect.DeepEqual(out, c.want) {
			t.Errorf("%d. %+v.DisplayCourses() = %+v; not %+v", i, c.db, out, c.want)
		}
	}
}
