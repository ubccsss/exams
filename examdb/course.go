package examdb

// Course represents a single course.
type Course struct {
	Code  string              `json:",omitempty"`
	Years map[int]*CourseYear `json:",omitempty"`
}

// FileCount returns the number of files for that course.
func (c Course) FileCount() int {
	count := 0
	for _, year := range c.Years {
		count += len(year.Files)
	}
	return count
}

// CourseYear contains all the files for a specific year of a course.
type CourseYear struct {
	Files []*File `json:",omitempty"`
}
