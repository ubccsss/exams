package exambot

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime"
	"net/http"
	_ "net/http/pprof"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/PuerkitoBio/goquery"
	"github.com/alecthomas/units"
	"github.com/d4l3k/docconv"
	archive "github.com/d4l3k/go-internetarchive"
	"github.com/temoto/robotstxt"
	"github.com/willf/bloom"
	backblaze "gopkg.in/kothar/go-backblaze.v0"

	piazza "github.com/d4l3k/piazza-api"

	"github.com/ubccsss/exams/db"
	"github.com/ubccsss/exams/exambot/exambotlib"
	"github.com/ubccsss/exams/workers"
)

const (
	userAgent = "UBC CSSS Exam Bot webmaster@ubccsss.org"

	// bloom filter configuration
	maxNumberOfPages  = 10000000
	falsePositiveRate = 0.00001

	toFetchBatchSize = 10000

	maxFileSize = 50 * units.MB
)

func compileRegexes(patterns ...string) []*regexp.Regexp {
	var r []*regexp.Regexp
	for _, pattern := range patterns {
		r = append(r, regexp.MustCompile(pattern))
	}
	return r
}

var (
	pageBucket            = []byte("pages")
	pageHashBucket        = []byte("pagehash")
	assortedBucket        = []byte("assorted")
	bloomFilterSeenKey    = []byte("bloom:seen")
	bloomFilterVisitedKey = []byte("bloom:visited")

	seedURLs = []db.Link{
		{URL: "https://www.cs.ubc.ca/~schmidtm/Courses/340-F16/"},
		{URL: "http://www.cs.ubc.ca/~pcarter/"},
		{URL: "https://sites.google.com/site/ubccpsc110/"},
		{URL: "https://www.cs.ubc.ca/our-department/people"},
		{URL: "https://ubccpsc.github.io"},
		{URL: "piazza://"},
	}

	archiveSearchPrefixes = []string{
		"https://www.ugrad.cs.ubc.ca/~",
		"https://www.cs.ubc.ca/~",
		"https://blogs.ubc.ca/cpsc",
		"https://www.cs.ubc.ca/people/",
	}

	blacklist = compileRegexes(
		".*//www.cs.ubc.ca/~davet/music/.*",
		".*\\?replytocom=.*",
		".*//www.cs.ubc.ca/bookings/.*",
		".*//www.cs.ubc.ca/news-events/calendar/.*",
		".*//www.cs.ubc.ca/print/.*",
		".*/bugzilla/.*",
		".*people/people/people/people.*",
		".*//sites.google.com/.*/system/errors/NodeNotFound.*",
		".*(aanb|acam|adhe|afst|agec|anae|anat|ansc|anth|apbi|appp|apsc|arbc|arc|arch|arcl|arst|arth|arts|asia|asic|asla|astr|astu|atsc|audi|ba|baac|babs|baen|bafi|bahc|bahr|baim|bait|bala|bama|bams|bapa|basc|basd|basm|batl|batm|baul|bioc|biof|biol|biot|bmeg|bota|brdg|busi|caps|ccfi|ccst|cdst|ceen|cell|cens|chbe|chem|chil|chin|cics|civl|clch|clst|cnps|cnrs|cnto|coec|cogs|cohr|comm|cons|cpen|crwr|csis|cspw|dani|dent|derm|dhyg|dmed|dpas|dsci|eced|econ|edcp|edst|educ|eece|elec|eli|emba|emer|ends|engl|enph|enpp|envr|eosc|epse|etec|exch|exgr|fact|febc|fhis|fipr|fish|fist|fmed|fmpr|fmst|fnel|fnh|fnis|food|fopr|fre|fren|frsi|frst|gbpr|gem|gene|geob|geog|germ|gpp|grek|grs|grsj|gsat|hebr|heso|hgse|hinu|hist|hpb|hunu|iar|iest|igen|inde|indo|inds|info|isci|ital|itst|iwme|japn|jrnl|kin|korn|lais|larc|laso|last|latn|law|lfs|libe|libr|ling|lled|math|mdvl|mech|medd|medg|medi|mgmt|micb|midw|mine|mrne|mtrl|musc|name|nest|neur|nrsc|nurs|obms|obst|ohs|onco|opth|ornt|orpa|paed|path|pcth|pers|phar|phil|phrm|phth|phyl|phys|plan|plnt|poli|pols|port|prin|psyc|psyt|punj|radi|relg|res|rgla|rhsc|rmst|rsot|russ|sans|scan|scie|seal|slav|soal|soci|soil|sowk|span|spha|spph|stat|sts|surg|swed|test|thtr|tibt|trsc|udes|ufor|ukrn|uro|urst|ursy|vant|vgrd|visa|vrhc|vurs|wood|wrds|writ|zool)\\d{3}.*",
	)

	validHosts = map[string]host{
		"www.cs.ubc.ca":       host{},
		"www.ugrad.cs.ubc.ca": host{},
		"blogs.ubc.ca":        host{},
		"sites.google.com": host{
			whitelist: compileRegexes(
				".*(cs|cpsc)\\d{3}.*",
			),
		},
		"ubccpsc.github.io": host{},
		"github.com": host{
			whitelist: compileRegexes(
				"^https://github.com/ubccpsc.*$",
			),
		},
	}
	scoreRegexes = map[int][]*regexp.Regexp{
		-1: compileRegexes(
			"final",
			"exam",
			"midterm",
			"sample",
			"mt",
			"(cs|cpsc)\\d{3}",
			"(20|19)\\d{2}",
		),
		1: compileRegexes(
			"report",
			"presentation",
			"thesis",
			"slide",
			"print",
		),
	}
)

type host struct {
	blacklist []*regexp.Regexp
	whitelist []*regexp.Regexp
}

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

// enforceUTF8 enforces that the string is valid UTF8.
func enforceUTF8(s string) string {
	if !utf8.ValidString(s) {
		v := make([]rune, 0, len(s))
		for i, r := range s {
			if r == utf8.RuneError {
				_, size := utf8.DecodeRuneInString(s[i:])
				if size == 1 {
					continue
				}
			}
			v = append(v, r)
		}
		s = string(v)
	}
	return s
}

var whitespaceRegexp = regexp.MustCompile(`\s+`)

// removeWhitespace replaces all whitespace with a single " ".
func removeWhitespace(s string) string {
	return whitespaceRegexp.ReplaceAllString(s, " ")
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
		match := pattern.MatchString(lower)
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

	for _, pattern := range blacklist {
		match := pattern.MatchString(lower)
		if match {
			return false, nil
		}
	}

	// Check against /robots.txt
	return validURLRobots(u)
}

// validURLRobots checks against the hosts /robots.txt if the URL is valid.
func validURLRobots(u *url.URL) (bool, error) {
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

type ContentTypeReader struct {
	buf    []byte
	reader io.Reader
}

func NewContentTypeReader(r io.Reader) (*ContentTypeReader, string, error) {
	c := ContentTypeReader{
		buf:    make([]byte, 512),
		reader: r,
	}
	n, err := r.Read(c.buf)
	if err != io.EOF && err != nil {
		return nil, "", err
	}
	c.buf = c.buf[:n]
	return &c, http.DetectContentType(c.buf), nil
}

func (r *ContentTypeReader) Read(w []byte) (int, error) {
	n := 0
	if len(r.buf) > 0 {
		n = copy(w, r.buf)
		r.buf = r.buf[n:]
		w = w[n:]
	}
	rn, err := r.reader.Read(w)
	if err != nil {
		return 0, err
	}
	return n + rn, nil
}

func urlBase(uri string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", err
	}
	return filepath.Base(u.Path), nil
}

// fetchURL returns all links from the page and the hash of the page.
func (s *Spider) fetchURL(tf db.ToFetch) (db.File, error) {
	u, err := url.Parse(tf.URL)
	if err != nil {
		return db.File{}, err
	}
	var reader io.Reader
	var statusCode int
	var givenContentType string
	if u.Scheme == piazza.PiazzaScheme {
		log.Printf("PIAZZA %s", tf.URL)
		resp, err := s.Piazza.Get(tf.URL)
		if err != nil {
			return db.File{}, err
		}
		reader = strings.NewReader(resp)
		givenContentType = "text/html; charset=utf-8"
		statusCode = 200
	} else {
		resp, err := makeGet(tf.URL)
		if err != nil {
			return db.File{}, err
		}
		defer resp.Body.Close()
		reader = resp.Body
		statusCode = resp.StatusCode
		givenContentType = resp.Header.Get("Content-Type")
	}

	reader = io.LimitReader(reader, int64(maxFileSize))

	f := db.File{
		SourceURL:   tf.URL,
		StatusCode:  statusCode,
		LastFetched: time.Now(),
		URLTitle:    tf.Title,
		RefererHash: tf.Source,
	}

	{
		var contentType string
		reader, contentType, err = NewContentTypeReader(reader)
		if err != nil {
			return db.File{}, err
		}
		if (contentType == "application/octet-stream" || contentType == "") && givenContentType != "" {
			contentType = givenContentType
		}
		f.ContentType = contentType
	}

	hasher := sha256.New()
	bodyReader := io.TeeReader(reader, hasher)

	mediaType, _, err := mime.ParseMediaType(f.ContentType)

	if mediaType == "text/html" || mediaType == "text/xml" {
		doc, err := goquery.NewDocumentFromReader(bodyReader)
		if err != nil {
			return db.File{}, err
		}

		f.Text = doc.Text()
		f.Title = doc.Find("title").Text()

		var links []db.Link
		doc.Find("a").Each(func(_ int, s *goquery.Selection) {
			title := s.Text()
			uri := s.AttrOr("href", "")

			ref, err := url.Parse(uri)
			if err != nil {
				log.Println(err)
				return
			}

			abs := u.ResolveReference(ref)
			abs.Fragment = ""
			link := abs.String()
			// Don't resolve relative to piazza://
			if !(strings.HasPrefix(link, piazza.PiazzaScheme) && !strings.HasPrefix(uri, piazza.PiazzaScheme)) {
				links = append(links, db.Link{
					Title: title,
					URL:   link,
				})
			}
		})
		f.Links = links
	} else if mediaType == "application/pdf" {
		body, err := ioutil.ReadAll(bodyReader)
		if err != nil {
			return db.File{}, err
		}
		text, meta, err := docconv.ConvertPDF(bytes.NewReader(body))
		if err != nil {
			log.Printf("failed to convert pdf: %+v", err)
		}
		f.Title = meta["Title"]
		f.Text = text
		f.Hash = hex.EncodeToString(hasher.Sum(nil))
		baseName, err := urlBase(f.URL)
		if err != nil {
			return db.File{}, err
		}
		fileName := f.Hash + "-" + baseName
		b2f, err := s.Bucket.UploadFile(fileName, nil, bytes.NewReader(body))
		if err != nil {
			return db.File{}, err
		}
		f.URL, err = s.Bucket.FileURL(b2f.Name)
		if err != nil {
			return db.File{}, err
		}
	} else {
		log.Printf("Unknown Content-Type! %q from %q", f.ContentType, f.SourceURL)
	}

	if _, err := io.Copy(ioutil.Discard, bodyReader); err != nil {
		return db.File{}, err
	}

	f.Text = removeWhitespace(enforceUTF8(f.Text))
	if len(f.Hash) == 0 {
		f.Hash = hex.EncodeToString(hasher.Sum(nil))
	}
	return f, nil
}

func (f *URLScore) computeScore() {
	path := strings.ToLower(f.URL)
	var score int
	for s, rs := range scoreRegexes {
		for _, r := range rs {
			if r.MatchString(path) {
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

// Spider is an exam spider.
type Spider struct {
	Mu struct {
		sync.RWMutex

		ToVisit []db.ToFetch

		Seen *bloom.BloomFilter
	}
	Bucket *backblaze.Bucket
	DB     *db.DB
	Piazza *piazza.HTMLWrapper
}

// MakeSpider makes a new spider.
func MakeSpider(db *db.DB, bucket *backblaze.Bucket, p *piazza.HTMLWrapper) *Spider {
	s := &Spider{
		DB:     db,
		Piazza: p,
		Bucket: bucket,
	}

	s.Mu.Seen = bloom.NewWithEstimates(maxNumberOfPages, falsePositiveRate)
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
	n, err := s.DB.ToFetchCount()
	if err != nil {
		log.Printf("stats error: %+v", err)
		return
	}
	log.Printf("Pending ToFetch %d", n)
}

type spiderState struct {
	ToVisit []URLScore
}

// Load loads the spider state.
func (s *Spider) Load() error {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	if err := s.DB.PopulateSeenVisited(s.Mu.Seen); err != nil {
		return err
	}

	return nil
}

var ErrNoMoreToFetches = errors.New("no ToFetches found")

func (s *Spider) GetURL() (db.ToFetch, error) {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	if len(s.Mu.ToVisit) == 0 {
		var err error
		s.Mu.ToVisit, err = s.DB.RandomToFetch(toFetchBatchSize)
		if err != nil {
			return db.ToFetch{}, err
		}
	}

	if len(s.Mu.ToVisit) == 0 {
		return db.ToFetch{}, ErrNoMoreToFetches
	}

	ret := s.Mu.ToVisit[0]
	s.Mu.ToVisit = s.Mu.ToVisit[1:]
	return ret, nil
}

// Worker ...
func (s *Spider) Worker() {
	for {
		tf, err := s.GetURL()
		if err == ErrNoMoreToFetches {
			log.Println("No URLs queued to visit!")
			time.Sleep(10 * time.Second)
			continue
		} else if err != nil {
			log.Printf("WORKER err: %+v", err)
			continue
		}

		page, err := s.fetchURL(tf)
		if err != nil {
			log.Printf("WORKER err: %+v", err)
			continue
		}

		if err := s.DB.SaveFile(&page); err != nil {
			log.Printf("%+v", page)
			log.Printf("%q %q %q", page.SourceURL, page.Title, page.Text)
			log.Printf("WORKER err: %+v", err)
			continue
		}

		if err := s.DB.DeleteToFetch(tf.URL); err != nil {
			log.Printf("WORKER err: %+v", err)
			continue
		}

		// Only follow links if 200 status code.
		if page.StatusCode == http.StatusOK {
			if err := s.AddAndExpandURLs(page, true); err != nil {
				log.Printf("WORKER err: %+v", err)
				continue
			}
		}
	}
}

func pageKey(url string) []byte {
	return []byte("page:" + url)
}
func hashKey(hash string) []byte {
	return []byte("pagehash:" + hash)
}

// AddAndExpandURLs cleans the URLs and adds them.
func (s *Spider) AddAndExpandURLs(f db.File, expand bool) error {
	added := 0
	var tfs []db.ToFetch

	for i, link := range f.Links {
		clean, err := cleanURL(link.URL)
		if err != nil {
			log.Printf("WORKER err: %s", err)
			continue
		}
		valid, err := validURL(clean)
		if err != nil {
			log.Printf("WORKER err: %s", err)
			continue
		}
		if !valid {
			continue
		}

		// don't expand piazza:// urls since we want to always visit the root ones
		if expand && !strings.HasPrefix(clean, "piazza") {
			expanded, err := exambotlib.ExpandURLToParents(clean)
			if err != nil {
				log.Printf("WORKER err: %s", err)
				continue
			}
			for _, url := range expanded {
				tfs = append(tfs, db.ToFetch{
					URL:    url,
					Title:  link.Title,
					Source: f.Hash,
				})
			}
		} else {
			tfs = append(tfs,
				db.ToFetch{
					URL:    clean,
					Title:  link.Title,
					Source: f.Hash,
				},
			)
		}

		if len(tfs) >= 1000 {
			n, err := s.AddURLs(tfs)
			if err != nil {
				return err
			}
			tfs = nil
			added += n
			log.Printf("AddAndExpandURLs: %q: added %d of %d processed. %d total.", f.SourceURL, added, i, len(f.Links))
		}
	}

	if len(tfs) == 0 {
		return nil
	}

	n, err := s.AddURLs(tfs)
	if err != nil {
		return err
	}
	added += n
	log.Printf("AddAndExpandURLs: %q: added %d of %d total.", f.SourceURL, added, len(f.Links))
	return nil
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
func (s *Spider) AddURLs(urls []db.ToFetch) (int, error) {
	var validTFs []db.ToFetch

	for _, url := range urls {
		s.Mu.Lock()
		seen := s.Mu.Seen.TestAndAddString(url.URL)
		s.Mu.Unlock()

		if seen && !alwaysVisit(url.URL) {
			continue
		}

		valid, err := validURL(url.URL)
		if err != nil {
			return 0, err
		}
		if !valid {
			continue
		}

		validTFs = append(validTFs, url)
	}

	return s.DB.BulkAddToFetch(validTFs)
}

var (
	piazzaUser = flag.String("piazzauser", "", "username of Piazza account to use for scraping")
	piazzaPass = flag.String("piazzapass", "", "password of Piazza account to use for scraping")
)

func Run(dbconn *db.DB, bucket *backblaze.Bucket) error {

	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	p, err := piazza.MakeClient(*piazzaUser, *piazzaPass)
	if err != nil {
		return err
	}

	s := MakeSpider(dbconn, bucket, p.HTMLWrapper())
	if err := s.Load(); err != nil {
		return err
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for range c {
			s.printStats()
			os.Exit(0)
		}
	}()

	log.Printf("Spinning up %d workers...", workers.Count)
	for i := 0; i < workers.Count; i++ {
		go s.Worker()
	}

	if err := s.AddAndExpandURLs(db.File{Links: seedURLs}, true); err != nil {
		return err
	}

	log.Println("Fetching seed data from internet archive...")
	for _, prefix := range archiveSearchPrefixes {
		log.Printf("... searching prefix %q", prefix)
		results := archive.SearchPrefix(prefix)
		var urls []db.Link
		for result := range results {
			urls = append(urls, db.Link{
				URL: result.OriginalURL,
			})
		}
		log.Printf("... adding %d urls", len(urls))
		if err := s.AddAndExpandURLs(db.File{Links: urls}, false); err != nil {
			return err
		}
	}

	return nil
}
