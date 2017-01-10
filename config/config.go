package config

import "github.com/alecthomas/units"

// Global configuration options.
const (
	StaticDir        = "static"
	ExamsDir         = StaticDir
	UploadedExamsDir = "uploaded"
	DBFile           = "data/exams.json"
	TemplateGlob     = "templates/*"
	ClassifierDir    = "data/classifiers"

	// MaxFileSize is the max size of a file that we'll handle.
	MaxFileSize = int64(10 * units.MB)
)
