package examsarchiveorg

import (
	"fmt"
	"log"
	"strings"

	archive "github.com/d4l3k/go-internetarchive"
	pcre "github.com/d4l3k/go-pcre"
)

var examRegex = pcre.MustCompile("(final|midterm|mt|exam(?!ple)|sample|practice).*\\.pdf$", 0)

func search(prefix string) []string {
	results := archive.SearchPrefix(prefix)
	dups := map[string]struct{}{}
	var deduped []string
	for result := range results {
		if result.Err != nil {
			log.Fatal(result.Err)
		}

		url := strings.ToLower(result.OriginalURL)
		if _, ok := dups[url]; !ok {
			if examRegex.FindIndex([]byte(url), 0) != nil {
				archiveURL := fmt.Sprintf("https://web.archive.org/web/2222/%s\n", url)
				log.Printf("Found %s", url)
				deduped = append(deduped, archiveURL)
			}
			dups[url] = struct{}{}
		}
	}
	return deduped
}

// PossibleExams returns possible exam files from archive.org.
func PossibleExams() []string {
	var results []string
	for _, prefix := range []string{"https://www.ugrad.cs.ubc.ca/~", "https://www.cs.ubc.ca/~"} {
		results = append(results, search(prefix)...)
	}
	log.Printf("Count %d", len(results))
	return results
}
