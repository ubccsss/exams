package config

import (
	"regexp"

	"github.com/alecthomas/units"
)

// Global configuration options.
var (
	StaticDir        = "static"
	ExamsDir         = StaticDir
	UploadedExamsDir = "uploaded"
	DBFile           = "data/exams.json"
	TemplateDir      = "templates"
	TemplateGlob     = TemplateDir + "/*"
	ClassifierDir    = "data/classifiers"

	// MaxFileSize is the max size of a file that we'll handle.
	MaxFileSize = int64(10 * units.MB)

	// Departments are the codes for the departments we support.
	Departments = []string{ComputerScience, Math, Law}

	// DisplayDepartment is a map for whether or not a department should be
	// displayed.
	DisplayDepartment = map[string]bool{
		ComputerScience: true,
	}

	// PDFRegexp is the regexp used to detect if a URL is a PDF.
	PDFRegexp = regexp.MustCompile(`\.pdf(\?.*)?$`)

	// ExamFirstPass is terms that indicate a PDF file is an exam.
	ExamFirstPass = []string{
		`final`,
		`midterm`,
		`sample`,
		`mt`,
		`practice`,
		`exam`,
		`m\ds\.pdf`,
		`\Wm\.pdf`,
		`\Wms\.pdf`,
	}
)

// Department codes
const (
	ComputerScience = "CPSC"
	Math            = "MATH"
	Law             = "LAW"
)
