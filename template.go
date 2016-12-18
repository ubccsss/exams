package main

import (
	"fmt"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

func renderTemplateExam(title, content string) string {
	title = strings.ToUpper(title)
	return renderTemplate(title, fmt.Sprintf(
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

func renderTemplate(title, content string) string {
	return fmt.Sprintf(layout, title, content)
}

func fetchTemplate() (string, error) {
	doc, err := goquery.NewDocument("https://ubccsss.org/services")
	if err != nil {
		return "", err
	}

	doc.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
		url := s.AttrOr("href", "")
		if strings.HasPrefix(url, "/") {
			s.SetAttr("href", "https://ubccsss.org"+url)
		}
	})

	title := doc.Find("title")
	parts := strings.Split(title.Text(), "|")
	title.ReplaceWithHtml("<title>%s |" + parts[len(parts)-1] + "</title>")

	section := doc.Find(".main-container .row > section")
	children := section.Children()
	children.First().ReplaceWithHtml(`%s`)
	children.Remove()
	return doc.Html()
}
