package main

import (
	"fmt"
	"log"
	"regexp"
	"strings"

	archive "github.com/d4l3k/go-internetarchive"
)

var examRegex = regexp.MustCompile("(final)")

func main() {
	results := archive.SearchPrefix("https://www.ugrad.cs.ubc.ca/~")
	dups := map[string]struct{}{}
	count := 0
	for result := range results {
		if result.Err != nil {
			log.Fatal(result.Err)
		}

		url := strings.ToLower(result.OriginalURL)
		if _, ok := dups[url]; !ok {
			if examRegex.MatchString(url) {
				fmt.Printf("https://web.archive.org/web/2222/%s\n", url)
				count++
			}
			dups[url] = struct{}{}
		}
	}
	log.Printf("Count %d", count)
}
