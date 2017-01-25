package exambotlib

import (
	"log"
	"net/url"
	"path"
	"regexp"
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

func ValidSuffix(uri string, validPostfix []string) bool {
	validExt := path.Ext(uri) == ""
	u, err := url.Parse(uri)
	if err == nil {
		uri = u.Path
	}
	for _, postfix := range validPostfix {
		if strings.HasSuffix(uri, postfix) {
			validExt = true
		}
	}
	if !validExt {
		var err error
		validExt, err = regexp.MatchString("^\\w*$", path.Base(uri))
		if err != nil {
			log.Fatal(err)
		}
	}
	return validExt
}
