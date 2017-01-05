package exambotlib

import (
	"net/url"
	"path"
	"strings"
)

func ExpandURLToParents(uri string) ([]string, error) {
	urls := []string{uri}
	parsed, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}
	for len(parsed.Path) > 1 {
		var parent string
		if len(parsed.RawQuery) == 0 {
			hasSlash := strings.HasSuffix(parsed.Path, "/")
			parsed.Path = path.Dir(parsed.Path)
			if hasSlash {
				continue
			}
			parent = parsed.String()
			if !strings.HasSuffix(parent, "/") {
				parent = parent + "/"
			}
		} else {
			parsed.RawQuery = ""
			parent = parsed.String()
		}
		if parent != uri {
			urls = append(urls, parent)
		}
	}
	return urls, nil
}
