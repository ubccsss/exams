package main

import (
	"container/heap"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	_ "net/http/pprof"
	"net/url"
	"os"
	"os/signal"
	"path"
	"regexp"
	"runtime"
	"runtime/pprof"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/boltdb/bolt"
	"github.com/d4l3k/exams/exambot/exambotlib"
	archive "github.com/d4l3k/go-internetarchive"
	piazza "github.com/d4l3k/piazza-api"
	"github.com/temoto/robotstxt"
	"github.com/willf/bloom"
)

const (
	userAgent   = "UBC CSSS Exam Bot vpc@ubccsss.org"
	workerCount = 8
	boltDBPath  = "exambot.boltdb"

	// bloom filter configuration
	maxNumberOfPages  = 1000000
	falsePositiveRate = 0.00001

	// shutdownTime is the number of seconds to shutdown after if there is no work
	// to be done.
	shutdownTime = 240
)

var (
	pageBucket            = []byte("pages")
	pageHashBucket        = []byte("pagehash")
	assortedBucket        = []byte("assorted")
	bloomFilterSeenKey    = []byte("bloom:seen")
	bloomFilterVisitedKey = []byte("bloom:visited")
	seedURLs              = []string{
		"https://www.cs.ubc.ca/~schmidtm/Courses/340-F16/",
		"http://www.cs.ubc.ca/~pcarter/",
		"https://sites.google.com/site/ubccpsc110/",
		"https://www.cs.ubc.ca/our-department/people",
		"https://ubccpsc.github.io",
		"piazza://",
	}
	archiveSearchPrefixes = []string{
		"https://www.ugrad.cs.ubc.ca/~",
		"https://www.cs.ubc.ca/~",
		"https://blogs.ubc.ca/cpsc",
		"https://www.cs.ubc.ca/people/",
	}
	validPostfix = []string{"/", ".html", ".htm", ".cgi", ".php"}
	blacklist    = []string{
		".*//www.cs.ubc.ca/~davet/music/.*",
		".*\\?replytocom=.*",
		".*//www.cs.ubc.ca/bookings/.*",
		".*//www.cs.ubc.ca/news-events/calendar/.*",
		".*//www.cs.ubc.ca/print/.*",
		".*/bugzilla/.*",
		".*people/people/people/people.*",
		".*//sites.google.com/.*/system/errors/NodeNotFound.*",
		".*(aanb|acam|adhe|afst|agec|anae|anat|ansc|anth|apbi|appp|apsc|arbc|arc|arch|arcl|arst|arth|arts|asia|asic|asla|astr|astu|atsc|audi|ba|baac|babs|baen|bafi|bahc|bahr|baim|bait|bala|bama|bams|bapa|basc|basd|basm|batl|batm|baul|bioc|biof|biol|biot|bmeg|bota|brdg|busi|caps|ccfi|ccst|cdst|ceen|cell|cens|chbe|chem|chil|chin|cics|civl|clch|clst|cnps|cnrs|cnto|coec|cogs|cohr|comm|cons|cpen|crwr|csis|cspw|dani|dent|derm|dhyg|dmed|dpas|dsci|eced|econ|edcp|edst|educ|eece|elec|eli|emba|emer|ends|engl|enph|enpp|envr|eosc|epse|etec|exch|exgr|fact|febc|fhis|fipr|fish|fist|fmed|fmpr|fmst|fnel|fnh|fnis|food|fopr|fre|fren|frsi|frst|gbpr|gem|gene|geob|geog|germ|gpp|grek|grs|grsj|gsat|hebr|heso|hgse|hinu|hist|hpb|hunu|iar|iest|igen|inde|indo|inds|info|isci|ital|itst|iwme|japn|jrnl|kin|korn|lais|larc|laso|last|latn|law|lfs|libe|libr|ling|lled|math|mdvl|mech|medd|medg|medi|mgmt|micb|midw|mine|mrne|mtrl|musc|name|nest|neur|nrsc|nurs|obms|obst|ohs|onco|opth|ornt|orpa|paed|path|pcth|pers|phar|phil|phrm|phth|phyl|phys|plan|plnt|poli|pols|port|prin|psyc|psyt|punj|radi|relg|res|rgla|rhsc|rmst|rsot|russ|sans|scan|scie|seal|slav|soal|soci|soil|sowk|span|spha|spph|stat|sts|surg|swed|test|thtr|tibt|trsc|udes|ufor|ukrn|uro|urst|ursy|vant|vgrd|visa|vrhc|vurs|wood|wrds|writ|zool)\\d{3}.*",
	}
	validHosts = map[string]host{
		"www.cs.ubc.ca":       host{},
		"www.ugrad.cs.ubc.ca": host{},
		"blogs.ubc.ca":        host{},
		"sites.google.com": host{
			whitelist: []string{
				".*(cs|cpsc)\\d{3}.*",
			},
		},
		"ubccpsc.github.io": host{},
		"github.com": host{
			whitelist: []string{
				"^https://github.com/ubccpsc.*$",
			},
		},
	}
	scoreRegexes = map[int][]string{
		-1: []string{"final", "exam", "midterm", "sample", "mt", "(cs|cpsc)\\d{3}", "(20|19)\\d{2}"},
		1:  []string{"report", "presentation", "thesis", "slide", "print"},
	}
)

type host struct {
	blacklist []string
	whitelist []string
}

var robotsCache = map[string]*robotstxt.Group{}
var robotsCacheLock sync.RWMutex

type Page struct {
	URL        string
	StatusCode int
	Hash       string
	Fetched    time.Time
	Links      []string
}

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

func cleanURL(uri string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", err
	}
	bits := strings.Split(u.Host, ":")
	if len(bits) <= 1 || len(bits[0]) == 0 {
		return uri, nil
	}
	host := bits[0]
	port := bits[1]
	if (u.Scheme == "https" && port == "443") || (u.Scheme == "http" && port == "80") {
		u.Host = host
	}
	return u.String(), nil
}

func validURL(uri string) (bool, error) {
	lower := strings.ToLower(uri)
	u, err := url.Parse(uri)
	if err != nil {
		return false, err
	}

	if u.Scheme == piazza.PiazzaScheme {
		return true, nil
	}

	// Check valid hosts
	hostRules, ok := validHosts[u.Host]
	if !ok {
		return false, nil
	}

	matchesWhiteList := len(hostRules.whitelist) == 0
	for _, pattern := range hostRules.whitelist {
		match, err := regexp.MatchString(pattern, lower)
		if err != nil {
			return false, err
		}
		if match {
			matchesWhiteList = true
			break
		}
	}
	if !matchesWhiteList {
		return false, nil
	}

	// Skip root indexes. "/", ""
	if len(u.Path) <= 1 {
		return false, nil
	}

	if !(validSuffix(u.Path) && validSuffix(u.RawQuery)) {
		return false, nil
	}

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
		if u.Host == "github.com" {
			robots = rhost.FindGroup("Googlebot")
		} else {
			robots = rhost.FindGroup(userAgent)
		}

		robotsCacheLock.Lock()
		robotsCache[u.Host] = robots
		robotsCacheLock.Unlock()
	}

	return robots.Test(u.Path), nil
}

// fetchURL returns all links from the page and the hash of the page.
func (s *Spider) fetchURL(uri string) (Page, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return Page{}, err
	}
	var reader io.Reader
	var statusCode int
	if u.Scheme == piazza.PiazzaScheme {
		log.Printf("PIAZZA %s", uri)
		resp, err := s.Piazza.Get(uri)
		if err != nil {
			return Page{}, err
		}
		reader = strings.NewReader(resp)
		statusCode = 200
	} else {
		resp, err := makeGet(uri)
		if err != nil {
			return Page{}, err
		}
		defer resp.Body.Close()
		reader = resp.Body
		statusCode = resp.StatusCode
	}

	hasher := sha1.New()
	bodyReader := io.TeeReader(reader, hasher)

	doc, err := goquery.NewDocumentFromReader(bodyReader)
	if err != nil {
		return Page{}, err
	}

	base, err := url.Parse(uri)
	if err != nil {
		return Page{}, err
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
		link := abs.String()
		// Don't resolve relative to piazza://
		if !(strings.HasPrefix(link, piazza.PiazzaScheme) && !strings.HasPrefix(uri, piazza.PiazzaScheme)) {
			links = append(links, link)
		}
	})

	hash := hex.EncodeToString(hasher.Sum(nil))
	return Page{
		URL:        uri,
		StatusCode: statusCode,
		Hash:       hash,
		Links:      links,
		Fetched:    time.Now(),
	}, nil
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
	Score int `json:",omitempty"`
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
		sync.RWMutex

		ToVisit    URLHeap
		ToVisitMap map[string]struct{}

		Seen    *bloom.BloomFilter
		Visited *bloom.BloomFilter
	}
	DB     *bolt.DB
	Piazza *piazza.HTMLWrapper
}

// MakeSpider makes a new spider.
func MakeSpider(db *bolt.DB, p *piazza.HTMLWrapper) *Spider {
	s := &Spider{
		DB:     db,
		Piazza: p,
	}

	s.Mu.ToVisitMap = map[string]struct{}{}
	s.Mu.Seen = bloom.NewWithEstimates(maxNumberOfPages, falsePositiveRate)
	s.Mu.Visited = bloom.NewWithEstimates(maxNumberOfPages, falsePositiveRate)
	log.Printf("Fetched bloom filter: m %d k %d", s.Mu.Seen.Cap(), s.Mu.Seen.K())

	go s.statsMonitor()

	return s
}

func (s *Spider) statsMonitor() {
	for range time.NewTicker(10 * time.Second).C {
		s.printStats()
	}
}

func (s *Spider) printStats() {
	s.Mu.RLock()
	s.Mu.RUnlock()
	log.Printf("ToVisit %d", len(s.Mu.ToVisit))
}

type spiderState struct {
	ToVisit URLHeap
}

// Save saves the spider state.
func (s *Spider) Save() error {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	if err := s.DB.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(assortedBucket)
		if err != nil {
			return err
		}
		v, err := s.Mu.Seen.MarshalJSON()
		if err != nil {
			return err
		}
		log.Printf("Seen bloom filter size = %d bytes", len(v))
		if err := b.Put(bloomFilterSeenKey, v); err != nil {
			return err
		}
		v, err = s.Mu.Visited.MarshalJSON()
		if err != nil {
			return err
		}
		log.Printf("Visited bloom filter size = %d bytes", len(v))
		if err := b.Put(bloomFilterVisitedKey, v); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}

	f, err := os.OpenFile("state.json", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	state := spiderState{
		s.Mu.ToVisit,
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(state); err != nil {
		return err
	}

	return nil
}

// Load loads the spider state.
func (s *Spider) Load() error {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	if err := s.DB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(assortedBucket)
		if b == nil {
			return errors.New("can't find bucket")
		}
		v := b.Get(bloomFilterSeenKey)
		if err := s.Mu.Seen.UnmarshalJSON(v); err != nil {
			return err
		}
		v = b.Get(bloomFilterVisitedKey)
		if err := s.Mu.Visited.UnmarshalJSON(v); err != nil {
			return err
		}
		return nil
	}); err != nil {
		log.Printf("ERR Loading Bloom Filter: %s", err)
	}

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
	for _, v := range s.Mu.ToVisit {
		s.Mu.ToVisitMap[v.URL] = struct{}{}
	}

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
				timeNoWork = time.Now().Add(shutdownTime * time.Second)
			}
			if time.Now().After(timeNoWork) {
				log.Printf("No work for %ds, shutting down.", shutdownTime)
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
		delete(s.Mu.ToVisitMap, url.URL)
		s.Mu.Unlock()

		valid, err := validURL(url.URL)
		if err != nil {
			log.Printf("WORKER err: %s", err)
			continue
		}

		if !valid {
			continue
		}

		page, err := s.fetchURL(url.URL)
		if err != nil {
			log.Printf("WORKER err: %s", err)
			continue
		}

		s.Mu.Lock()
		visited := s.Mu.Visited.TestAndAddString(page.Hash)
		s.Mu.Unlock()

		if visited && !alwaysVisit(url.URL) {
			continue
		}

		if err := s.savePage(page); err != nil {
			log.Printf("WORKER err: %s", err)
			continue
		}

		// Only follow links if 200 status code.
		if page.StatusCode == 200 {
			s.AddAndExpandURLs(page.Links, true)
		}

		lastNoWork = false
	}
}

func pageKey(url string) []byte {
	return []byte("page:" + url)
}
func hashKey(hash string) []byte {
	return []byte("pagehash:" + hash)
}

func (s *Spider) savePage(p Page) error {
	json, err := p.marshal()
	if err != nil {
		return err
	}
	pagekey := pageKey(p.URL)
	hashkey := hashKey(p.Hash)
	if err := s.DB.Update(func(tx *bolt.Tx) error {
		pagebucket, err := tx.CreateBucketIfNotExists(pageBucket)
		if err != nil {
			return err
		}
		if err := pagebucket.Put(pagekey, json); err != nil {
			return err
		}
		hashbucket, err := tx.CreateBucketIfNotExists(pageHashBucket)
		if err != nil {
			return err
		}
		if err := hashbucket.Put(hashkey, pagekey); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

func (p Page) marshal() ([]byte, error) {
	return json.Marshal(p)
}

// AddAndExpandURLs cleans the URLs and adds them.
func (s *Spider) AddAndExpandURLs(urls []string, expand bool) {
	// Expand valid URLs
	added := 0
	for i, u := range urls {
		if i%1000 == 0 && i > 0 {
			log.Printf("AddAndExpandURLs: added %d of %d processed. Total: %d", added, i, len(urls))
		}
		clean, err := cleanURL(u)
		if err != nil {
			log.Printf("WORKER err: %s", err)
			continue
		}
		valid, err := validURL(clean)
		if err != nil {
			log.Printf("WORKER err: %s", err)
			continue
		}
		if valid {
			// don't expand piazza:// urls since we want to always visit the root ones
			if expand && !strings.HasPrefix(clean, "piazza") {
				expanded, err := exambotlib.ExpandURLToParents(clean)
				if err != nil {
					log.Printf("WORKER err: %s", err)
					continue
				}
				added += s.AddURLs(expanded)
			} else {
				added += s.AddURLs([]string{clean})
			}
		}
	}
}

func alwaysVisit(uri string) bool {
	u, _ := url.Parse(uri)
	if u.Scheme == piazza.PiazzaScheme {
		return len(u.Path) <= 1
	}
	return false
}

// AddURLs adds a bunch of URLs to be processed if valid and returns how many
// were added.
func (s *Spider) AddURLs(urls []string) int {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	added := 0
	for _, url := range urls {
		if s.Mu.Seen.TestAndAddString(url) && !alwaysVisit(url) {
			continue
		}

		fmt.Fprintf(s.Out, "%s\n", url)
		valid, err := validURL(url)
		if err != nil {
			log.Printf("add URL err: %s", err)
			continue
		}
		if !valid {
			continue
		}
		// Don't add a URL multiple times.
		if _, ok := s.Mu.ToVisitMap[url]; ok {
			continue
		}
		us := &URLScore{URL: url}
		us.computeScore()
		heap.Push(&s.Mu.ToVisit, us)
		s.Mu.ToVisitMap[url] = struct{}{}
		added++
	}
	return added
}

var (
	cpuprofile = flag.String("cpuprofile", "", "write cpu profile `file`")
	memprofile = flag.String("memprofile", "", "write memory profile to `file`")

	piazzaUser = flag.String("piazzauser", "", "username of Piazza account to use for scraping")
	piazzaPass = flag.String("piazzapass", "", "password of Piazza account to use for scraping")
)

func main() {
	flag.Parse()
	log.SetOutput(os.Stderr)

	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal("could not create CPU profile: ", err)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatal("could not start CPU profile: ", err)
		}
		defer pprof.StopCPUProfile()
	}

	db, err := bolt.Open(boltDBPath, 0600, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	args := flag.Args()
	if len(args) == 1 {
		switch args[0] {
		case "list":
			if err := db.View(func(tx *bolt.Tx) error {
				b := tx.Bucket(pageBucket)
				if b == nil {
					return errors.New("can't find bucket")
				}

				seen := bloom.NewWithEstimates(maxNumberOfPages, falsePositiveRate)
				var page Page
				var count int

				if err := b.ForEach(func(k, v []byte) error {
					count++
					if err := json.Unmarshal(v, &page); err != nil {
						return err
					}
					if !seen.TestAndAddString(page.URL) {
						fmt.Println(page.URL)
					}

					for _, l := range page.Links {
						if !seen.TestAndAddString(l) {
							fmt.Println(l)
						}
					}
					if count%10000 == 0 {
						log.Printf("Count %d", count)
					}
					return nil
				}); err != nil {
					return err
				}

				return nil
			}); err != nil {
				log.Fatal(err)
			}
			return
		}
	}

	p, err := piazza.MakeClient(*piazzaUser, *piazzaPass)
	if err != nil {
		log.Fatal(err)
	}

	s := MakeSpider(db, p.HTMLWrapper())
	if err := s.Load(); err != nil {
		log.Print(err)
	}

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
			pprof.StopCPUProfile()
			if *memprofile != "" {
				f, err := os.Create(*memprofile)
				if err != nil {
					log.Fatal("could not create memory profile: ", err)
				}
				runtime.GC() // get up-to-date statistics
				if err := pprof.WriteHeapProfile(f); err != nil {
					log.Fatal("could not write memory profile: ", err)
				}
				f.Close()
			}
			s.printStats()
			os.Exit(0)
		}
	}()

	s.AddAndExpandURLs(seedURLs, true)
	log.Println("Fetching seed data from internet archive...")
	for _, prefix := range archiveSearchPrefixes {
		log.Printf("... searching prefix %q", prefix)
		results := archive.SearchPrefix(prefix)
		var urls []string
		for result := range results {
			urls = append(urls, result.OriginalURL)
		}
		log.Printf("... adding %d urls", len(urls))
		go s.AddAndExpandURLs(urls, false)
	}

	log.Printf("Spinning up %d workers...", workerCount)
	for i := 0; i < workerCount-1; i++ {
		go s.Worker()
	}
	s.Worker()
}
