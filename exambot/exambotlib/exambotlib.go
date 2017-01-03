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
	for parsed.Path != "/" {
		parsed.Path = path.Dir(parsed.Path)
		parent := parsed.String()
		if !strings.HasSuffix(parent, "/") {
			parent = parent + "/"
		}
		if parent != uri {
			urls = append(urls, parent)
		}
	}
	return urls, nil
}
