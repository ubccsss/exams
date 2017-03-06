package examdb

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/ubccsss/exams/config"
)

// Course represents a single course.
type Course struct {
	// Code should always be lowercase.
	Code string `json:",omitempty"`
	Desc string `json:",omitempty"`
}

// Department returns the department code for the course.
func (c Course) Department() string {
	code := strings.ToLower(c.Code)
	for _, dept := range config.Departments {
		if strings.HasPrefix(code, strings.ToLower(dept)) {
			return dept
		}
	}
	if strings.HasPrefix(c.Code, "cs") {
		return config.ComputerScience
	}
	return ""
}

// AlternateIDs returns the possible ID formats.
func (c Course) AlternateIDs() []string {
	dept := c.Department()
	number := c.Number()
	if dept == "" || number < 0 {
		return nil
	}

	ids := []string{
		c.Code,
		fmt.Sprintf("%s%d", dept, number),
		fmt.Sprintf("%s-%d", dept, number),
	}
	if dept == config.ComputerScience {
		ids = append(ids, strconv.Itoa(number))
	}

	properFormat := fmt.Sprintf("%s %d", dept, number)
	if !strings.EqualFold(c.Code, properFormat) {
		ids = append(ids, properFormat)
	}

	for i, id := range ids {
		ids[i] = strings.ToLower(id)
	}
	return ids
}

// Number returns the course number.
func (c Course) Number() int {
	number, err := strconv.Atoi(numberRegexp.FindString(c.Code))
	if err != nil {
		return -1
	}
	return number
}

var numberRegexp = regexp.MustCompile(`\d+`)

// YearLevel returns a string representing the year level in the form "x00".
func (c Course) YearLevel() string {
	dept := c.Department()
	number := c.Number()
	if dept == "" || number < 0 {
		return ""
	}

	// Zero out last two digits.
	number = (number / 100) * 100
	return strings.ToUpper(fmt.Sprintf("%s %.3d", dept, number))
}
