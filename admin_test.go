package main

import (
	"testing"
	"time"

	"github.com/ubccsss/exams/examdb"
)

func TestSkipInfer(t *testing.T) {
	cases := []struct {
		f      examdb.File
		always bool
		want   bool
	}{
		{
			examdb.File{},
			false,
			false,
		},
		{
			examdb.File{
				Inferred: &examdb.File{
					NotAnExam: true,
				},
			},
			false,
			true,
		},
		{
			examdb.File{
				Inferred: &examdb.File{
					Name: "foo",
				},
			},
			false,
			true,
		},
		{
			examdb.File{
				Inferred: &examdb.File{
					Name: "foo",
				},
			},
			true,
			false,
		},
		{
			examdb.File{
				Inferred: &examdb.File{
					Name:    "foo",
					Updated: time.Now(),
				},
			},
			true,
			true,
		},
		{
			examdb.File{
				NotAnExam: true,
			},
			true,
			true,
		},
		{
			examdb.File{
				LastResponseCode: 404,
			},
			true,
			true,
		},
		{
			examdb.File{
				LastResponseCode: 200,
			},
			true,
			false,
		},
		{
			examdb.File{
				HandClassified: true,
			},
			true,
			true,
		},
	}

	for i, c := range cases {
		out := skipInfer(&c.f, c.always)
		if out != c.want {
			t.Errorf("%d. skipInfer(%+v, %+v) = %+v; not %+v", i, c.f, c.always, out, c.want)
		}
	}
}
