package examdb

// Course represents a single course.
type Course struct {
	Code  string
	Years map[int]*CourseYear
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
	Files []*File
}
