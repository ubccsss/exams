package examdb

import "strings"

// Course represents a single course.
type Course struct {
	Code string `json:",omitempty"`
	Desc string `json:",omitempty"`
}

// AlternateIDs returns the possible ID formats.
func (c Course) AlternateIDs() []string {
	if len(c.Code) == 0 {
		return []string{}
	}
	if len(c.Code) < 5 {
		return []string{c.Code}
	}
	number := c.Code[2:]
	return []string{c.Code, "cpsc" + number, "cpsc-" + number, number}
}

// YearLevel returns a string representing the year level in the form "x00".
func (c Course) YearLevel() string {
	if len(c.Code) < 5 {
		return ""
	}
	return strings.ToUpper(c.Code[0:3] + "00")
}
