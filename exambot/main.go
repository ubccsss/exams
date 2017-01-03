package main

import (
	"container/heap"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/d4l3k/exams/exambot/exambotlib"
	archive "github.com/d4l3k/go-internetarchive"
	"github.com/temoto/robotstxt"
)

const (
	userAgent   = "UBC CSSS Exam Bot vpc@ubccsss.org"
	workerCount = 8
)

var (
	seedURLs = []string{
		"https://www.cs.ubc.ca/~schmidtm/Courses/340-F16/",
		"http://www.cs.ubc.ca/~pcarter/",
		"https://sites.google.com/site/ubccpsc110/",
	}
	archiveSearchPrefixes = []string{
		"https://www.ugrad.cs.ubc.ca/~",
		"https://www.cs.ubc.ca/~",
		"https://blogs.ubc.ca/cpsc",
	}
	validPostfix = []string{"/", ".html", ".htm", ".cgi", ".php"}
	blacklist    = []string{".*//www.cs.ubc.ca/~davet/music/.*"}
	validHosts   = map[string]struct{}{
		"www.cs.ubc.ca":       struct{}{},
		"www.ugrad.cs.ubc.ca": struct{}{},
		"blogs.ubc.ca":        struct{}{},
		"sites.google.com":    struct{}{},
	}
	scoreRegexes = map[int][]string{
		-1: []string{"final", "exam", "midterm", "sample", "mt", "(cs|cpsc)\\d{3}", "(20|19)\\d{2}"},
		1:  []string{"report", "presentation", "thesis", "slide"},
	}
)

var robotsCache = map[string]*robotstxt.Group{}
var robotsCacheLock sync.RWMutex

func makeGet(url string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("GET err %s: %s", url, err)
		return nil, err
	}
	log.Printf("GET %d %s", resp.StatusCode, url)
	return resp, nil
}

func validSuffix(uri string) bool {
	validExt := path.Ext(uri) == ""
	for _, postfix := range validPostfix {
		if strings.HasSuffix(uri, postfix) {
			validExt = true
		}
	}
	return validExt
}

func validURL(uri string) (bool, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return false, err
	}

	// Check valid hosts
	if _, ok := validHosts[u.Host]; ok {
		return false, nil
	}

	// Only want /~foo paths since that's where the courses are.
	if !strings.HasPrefix(u.Path, "/~") {
		return false, nil
	}

	if !(validSuffix(u.Path) && validSuffix(u.RawQuery)) {
		return false, nil
	}

	lower := strings.ToLower(uri)
	for _, pattern := range blacklist {
		match, err := regexp.MatchString(pattern, lower)
		if err != nil {
			return false, err
		}
		if match {
			return false, nil
		}
	}

	// Check against /robots.txt
	robotsCacheLock.RLock()
	robots, ok := robotsCache[u.Host]
	robotsCacheLock.RUnlock()

	if !ok {
		resp, err := makeGet(fmt.Sprintf("https://%s/robots.txt", u.Host))
		if err != nil {
			return false, nil
		}
		defer resp.Body.Close()
		rhost, err := robotstxt.FromResponse(resp)
		if err != nil {
			return false, nil
		}
		robots = rhost.FindGroup(userAgent)

		robotsCacheLock.Lock()
		robotsCache[u.Host] = robots
		robotsCacheLock.Unlock()
	}

	return robots.Test(u.Path), nil
}

// fetchURL returns all links from the page and the hash of the page.
func fetchURL(uri string) ([]string, string, error) {
	resp, err := makeGet(uri)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	hasher := sha1.New()
	bodyReader := io.TeeReader(resp.Body, hasher)

	doc, err := goquery.NewDocumentFromReader(bodyReader)
	if err != nil {
		return nil, "", err
	}

	base, err := url.Parse(uri)
	if err != nil {
		return nil, "", err
	}

	var links []string
	doc.Find("a").Each(func(_ int, s *goquery.Selection) {
		uri := s.AttrOr("href", "")

		ref, err := url.Parse(uri)
		if err != nil {
			log.Println(err)
			return
		}

		abs := base.ResolveReference(ref)
		abs.Fragment = ""
		links = append(links, abs.String())
	})

	hash := hex.EncodeToString(hasher.Sum(nil))

	return links, hash, nil
}

var regexCache = map[string]*regexp.Regexp{}

func regexpMatch(pattern, path string) bool {
	r, ok := regexCache[pattern]
	if !ok {
		r = regexp.MustCompile(pattern)
		regexCache[pattern] = r
	}

	return r.FindIndex([]byte(path)) != nil
}

func (f *URLScore) computeScore() {
	path := strings.ToLower(f.URL)
	var score int
	for s, rs := range scoreRegexes {
		for _, r := range rs {
			if regexpMatch(r, path) {
				score += s
			}
		}
	}
	f.Score = score
}

// URLScore is a single URL and a score
type URLScore struct {
	URL   string
	Score int
}

// An URLHeap is a min-heap of strings.
type URLHeap []*URLScore

func (h URLHeap) Len() int           { return len(h) }
func (h URLHeap) Less(i, j int) bool { return h[i].Score < h[j].Score }
func (h URLHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

// Push ...
func (h *URLHeap) Push(x interface{}) {
	// Push and Pop use pointer receivers because they modify the slice's length,
	// not just its contents.
	*h = append(*h, x.(*URLScore))
}

// Pop ...
func (h *URLHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

// Spider is an exam spider.
type Spider struct {
	Out io.WriteCloser
	Mu  struct {
		ToVisit URLHeap
		Visited map[string]struct{}
		Seen    map[string]struct{}
		sync.RWMutex
	}
	HashMu struct {
		ProcessedHashes map[string]struct{}
		sync.RWMutex
	}
}

// MakeSpider makes a new spider.
func MakeSpider() *Spider {
	s := &Spider{}
	s.Mu.Visited = map[string]struct{}{}
	s.Mu.Seen = map[string]struct{}{}
	s.HashMu.ProcessedHashes = map[string]struct{}{}
	return s
}

type spiderState struct {
	ToVisit         URLHeap
	Visited         map[string]struct{}
	Seen            map[string]struct{}
	ProcessedHashes map[string]struct{}
}

// Save saves the spider state.
func (s *Spider) Save() error {
	s.Mu.Lock()
	s.HashMu.Lock()
	defer s.Mu.Unlock()
	defer s.HashMu.Unlock()

	f, err := os.OpenFile("state.json", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	state := spiderState{
		s.Mu.ToVisit,
		s.Mu.Visited,
		s.Mu.Seen,
		s.HashMu.ProcessedHashes,
	}
	if err := json.NewEncoder(f).Encode(state); err != nil {
		return err
	}

	return nil
}

// Load loads the spider state.
func (s *Spider) Load() error {
	s.Mu.Lock()
	s.HashMu.Lock()
	defer s.Mu.Unlock()
	defer s.HashMu.Unlock()

	f, err := os.Open("state.json")
	if err != nil {
		return err
	}
	defer f.Close()

	state := spiderState{}
	if err := json.NewDecoder(f).Decode(&state); err != nil {
		return err
	}

	s.Mu.ToVisit = state.ToVisit
	s.Mu.Visited = state.Visited
	s.Mu.Seen = state.Seen
	s.HashMu.ProcessedHashes = state.ProcessedHashes

	return nil
}

// Worker ...
func (s *Spider) Worker() {
	var lastNoWork bool
	var timeNoWork time.Time
	for {
		if len(s.Mu.ToVisit) == 0 {
			if !lastNoWork {
				log.Println("No URLs queued to visit!")
				lastNoWork = true
				timeNoWork = time.Now().Add(15 * time.Second)
			}
			if time.Now().After(timeNoWork) {
				log.Printf("No work for 15s, shutting down.")
				os.Exit(0)
			}
			time.Sleep(1 * time.Second)
			continue
		}

		s.Mu.Lock()
		if len(s.Mu.ToVisit) == 0 {
			s.Mu.Unlock()
			continue
		}
		url := heap.Pop(&s.Mu.ToVisit).(*URLScore)
		s.Mu.Visited[url.URL] = struct{}{}
		s.Mu.Unlock()

		urls, hash, err := fetchURL(url.URL)
		if err != nil {
			log.Printf("WORKER err: %s", err)
			continue
		}

		s.HashMu.Lock()
		if _, ok := s.HashMu.ProcessedHashes[hash]; ok {
			s.HashMu.Unlock()
			continue
		}
		s.HashMu.ProcessedHashes[hash] = struct{}{}
		s.HashMu.Unlock()

		s.AddAndExpandURLs(urls)
		lastNoWork = false
	}
}

func (s *Spider) AddAndExpandURLs(urls []string) {
	// Expand valid URLs
	expandedURLs := urls
	for _, u := range urls {
		valid, err := validURL(u)
		if err != nil {
			log.Printf("WORKER err: %s", err)
			continue
		}
		if valid {
			expanded, err := exambotlib.ExpandURLToParents(u)
			if err != nil {
				log.Printf("WORKER err: %s", err)
				continue
			}
			expandedURLs = append(expandedURLs, expanded...)
		}
	}
	s.AddURLs(expandedURLs)
}

// AddURLs adds a bunch of URLs to be processed if valid.
func (s *Spider) AddURLs(urls []string) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	for _, url := range urls {
		if _, ok := s.Mu.Visited[url]; ok {
			continue
		}
		if _, ok := s.Mu.Seen[url]; ok {
			continue
		}
		s.Mu.Seen[url] = struct{}{}
		fmt.Fprintf(s.Out, "%s\n", url)
		valid, err := validURL(url)
		if err != nil {
			log.Printf("add URL err: %s", err)
			continue
		}
		if !valid {
			continue
		}
		//log.Printf("+ %s", url)
		us := &URLScore{URL: url}
		us.computeScore()
		heap.Push(&s.Mu.ToVisit, us)
	}
}

func main() {
	log.SetOutput(os.Stderr)

	s := MakeSpider()
	if err := s.Load(); err != nil {
		log.Print(err)
	}

	var err error
	if _, err2 := os.Stat("index.txt"); os.IsNotExist(err2) {
		s.Out, err = os.Create("index.txt")
	} else if err != nil {
		log.Fatal(err)
	} else {
		s.Out, err = os.OpenFile("index.txt", os.O_APPEND|os.O_WRONLY, 0600)
	}
	if err != nil {
		log.Fatal(err)
	}
	defer s.Out.Close()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for _ = range c {
			log.Println("Saving progress...")
			if err := s.Save(); err != nil {
				log.Fatal(err)
			}
			os.Exit(0)
		}
	}()

	log.Println("Fetching seed data from internet archive...")
	urls := seedURLs
	for _, prefix := range archiveSearchPrefixes {
		results := archive.SearchPrefix(prefix)
		for result := range results {
			urls = append(urls, result.OriginalURL)
		}
		s.AddAndExpandURLs(urls)
	}
	log.Printf("Spinning up %d workers...", workerCount)
	for i := 0; i < workerCount-1; i++ {
		go s.Worker()
	}
	s.Worker()
}
