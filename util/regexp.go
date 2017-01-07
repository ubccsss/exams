package util

import (
	"regexp"
	"sync"
)

var (
	regexCache   = map[string]*regexp.Regexp{}
	regexCatchMu sync.RWMutex
)

// RegexpMatch compiles a regexp and stores it in a cache for later iterations.
func RegexpMatch(pattern, path string) bool {
	regexCatchMu.RLock()
	r, ok := regexCache[pattern]
	regexCatchMu.RUnlock()
	if !ok {
		r = regexp.MustCompile(pattern)
		regexCatchMu.Lock()
		regexCache[pattern] = r
		regexCatchMu.Unlock()
	}

	return r.FindIndex([]byte(path)) != nil
}
