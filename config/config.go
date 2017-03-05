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
)

// PDFRegexp is the regexp used to detect if a URL is a PDF.
var PDFRegexp = regexp.MustCompile(`\.pdf(\?.*)?$`)

// ExamFirstPass is terms that indicate a PDF file is an exam.
var ExamFirstPass = []string{
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
