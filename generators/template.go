package generators

import (
	"bytes"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/howeyc/fsnotify"
	"github.com/russross/blackfriday"
	"github.com/ubccsss/exams/config"
)

// Templates are all of the HTML templates needed.
var Templates *template.Template

var templateFuncs = template.FuncMap{
	"pathToURL": func(fp string) string {
		base := path.Base(fp)
		rest := path.Dir(fp)
		return path.Join("/", rest, url.PathEscape(base))
	},
}

func updateTemplates() {
	log.Printf("%s/: changed! Loading templates...", config.TemplateDir)
	t := template.New("templates")
	t.Funcs(templateFuncs)
	template.Must(t.ParseGlob(config.TemplateGlob))
	Templates = t
}

func updateTemplatesDebounced() chan struct{} {
	ch := make(chan struct{})
	var timer *time.Timer

	go func() {
		for range ch {
			if timer != nil {
				timer.Stop()
			}
			timer = time.AfterFunc(300*time.Millisecond, updateTemplates)
		}
	}()

	return ch
}

func init() {
	updateTemplates()
	update := updateTemplatesDebounced()

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		for {
			select {
			case <-watcher.Event:
				update <- struct{}{}
			case err := <-watcher.Error:
				log.Println("Watch error:", err)
			}
		}
	}()

	if err := watcher.Watch(config.TemplateDir); err != nil {
		log.Fatal(err)
	}
}

func (g *Generator) renderTemplateExam(title, content string) string {
	title = strings.ToUpper(title)
	return g.renderTemplate(title, fmt.Sprintf(
		`<ol class="breadcrumb"><li><a href="..">Exams Database</a></li>
		<li class="active">%s</li>
		</ol>
		%s
		<div id="book-navigation-1440" class="book-navigation">
		<ul class="pager clearfix">
		<li><a href=".." class="page-up" title="Go to parent page">up</a></li>
		</ul>
		</div>`, title, content))
}

func (g *Generator) renderTemplate(title, content string) string {
	if err := g.fetchLayout(); err != nil {
		log.Println(err)
	}
	return fmt.Sprintf(g.layout, title, content)
}

const templateURL = "https://ubccsss.org/services/"

var importRegexp = regexp.MustCompile("@import .*;")

func (g *Generator) fetchLayout() error {
	var err error

	g.layoutOnce.Do(func() {
		start := time.Now()
		log.Printf("Fetching layout template from %q", templateURL)
		base, err2 := url.Parse(templateURL)
		if err2 != nil {
			err = err2
			return
		}
		doc, err2 := goquery.NewDocument(templateURL)
		if err2 != nil {
			err = err2
			return
		}

		// Clean metadata.
		doc.Find(`link[rel="shortlink"], link[rel="canonical"], meta[name="Generator"]`).Remove()

		// Resolve URLs.
		doc.Find("a[href], link[href]").Each(func(_ int, s *goquery.Selection) {
			raw := s.AttrOr("href", "")
			// Don't resolve cloud flare email protection.
			if strings.HasPrefix(raw, "/cdn-cgi/l/email-protection") {
				return
			}
			url, err := url.Parse(raw)
			if err != nil {
				return
			}
			resolved := base.ResolveReference(url)
			s.SetAttr("href", resolved.String())
		})

		doc.Find("script[src], img[src]").Each(func(_ int, s *goquery.Selection) {
			url, err2 := url.Parse(s.AttrOr("src", ""))
			if err2 != nil {
				err = err2
				return
			}
			resolved := base.ResolveReference(url)
			s.SetAttr("src", resolved.String())
		})

		var importBuf bytes.Buffer
		var buf bytes.Buffer

		// Package all CSS and scripts into one file.
		stylesheets := doc.Find(`link[href][rel="stylesheet"]`)
		stylesheets.Each(func(_ int, s *goquery.Selection) {
			resp, err2 := http.Get(s.AttrOr("href", ""))
			if err2 != nil {
				err = err2
				return
			}
			defer resp.Body.Close()
			body, _ := ioutil.ReadAll(resp.Body)
			lastIdx := 0
			for _, match := range importRegexp.FindAllIndex(body, -1) {
				buf.Write(body[lastIdx:match[0]])
				importBuf.Write(body[match[0]:match[1]])
				lastIdx = match[1]
			}
			buf.Write(body[lastIdx:len(body)])
			buf.WriteRune('\n')
		})
		if err != nil {
			return
		}

		stylesheet := stylesheets.First()
		stylesheet.SetAttr("href", "/style.css")
		stylesheet.RemoveAttr("integrity")
		stylesheets.Slice(1, stylesheets.Length()).Remove()

		if _, err = buf.WriteTo(&importBuf); err != nil {
			return
		}

		if err = ioutil.WriteFile(path.Join(g.examsDir, "style.css"), importBuf.Bytes(), 0755); err != nil {
			return
		}

		importBuf.Reset()
		buf.Reset()

		scripts := doc.Find(`script[src]`)
		scripts.Each(func(_ int, s *goquery.Selection) {
			resp, err2 := http.Get(s.AttrOr("src", ""))
			if err2 != nil {
				err = err2
				return
			}
			defer resp.Body.Close()
			buf.ReadFrom(resp.Body)
			buf.WriteRune('\n')
		})
		if err != nil {
			return
		}

		scripts.First().SetAttr("src", "/scripts.js")
		scripts.Slice(1, scripts.Length()).Remove()

		if err = ioutil.WriteFile(path.Join(g.examsDir, "scripts.js"), buf.Bytes(), 0755); err != nil {
			return
		}

		title := doc.Find("title")
		parts := strings.Split(title.Text(), "|")
		title.ReplaceWithHtml("<title>%s |" + parts[len(parts)-1] + "</title>")

		section := doc.Find("body > .container .row")
		children := section.Children()
		children.First().ReplaceWithHtml(`<div>%s</div>`)
		children.Remove()

		layout, err2 := doc.Html()
		if err2 != nil {
			err = err2
			return
		}

		// This replaces the ubccsss.org Google Analytics code with the one for
		// exams.ubccsss.org.
		layout = strings.Replace(layout, "UA-88004303-1", "UA-88004303-3", -1)

		g.layout = layout
		log.Printf("Fetched layout template. Took %s", time.Since(start))
	})

	return err
}

// ExecuteTemplate runs a template and writes it to w.
func ExecuteTemplate(w http.ResponseWriter, name string, data interface{}) error {
	var buf bytes.Buffer
	if err := Templates.ExecuteTemplate(&buf, name, data); err != nil {
		return err
	}

	if path.Ext(name) == ".md" {
		w.Write([]byte(`<div class="container">`))
		w.Write([]byte(blackfriday.MarkdownCommon(buf.Bytes())))
		w.Write([]byte(`</div>`))
		return nil
	}

	_, err := buf.WriteTo(w)
	return err
}
