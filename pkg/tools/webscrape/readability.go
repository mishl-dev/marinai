package webscrape

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/PuerkitoBio/goquery"
)

type ReadabilityExtractor struct {
	minContentLength int
	minScore         float64
}

func NewReadabilityExtractor() *ReadabilityExtractor {
	return &ReadabilityExtractor{
		minContentLength: 140,
		minScore:         20,
	}
}

type Article struct {
	Title       string
	Content     string
	TextContent string
	Author      string
	Excerpt     string
	SiteName    string
}

func (r *ReadabilityExtractor) Extract(html string) (*Article, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	article := &Article{}

	article.Title = r.extractTitle(doc)

	r.extractMetadata(doc, article)

	r.prepDocument(doc)

	article.Content = r.extractContent(doc)
	article.TextContent = r.htmlToText(article.Content)

	if article.Excerpt == "" && len(article.TextContent) > 0 {
		article.Excerpt = r.truncate(article.TextContent, 200)
	}

	return article, nil
}

func (r *ReadabilityExtractor) extractTitle(doc *goquery.Document) string {
	title := doc.Find("title").Text()
	title = strings.TrimSpace(title)

	parts := strings.Split(title, " - ")
	if len(parts) > 1 {
		title = parts[0]
	}

	parts = strings.Split(title, " | ")
	if len(parts) > 1 {
		title = parts[0]
	}

	return title
}

func (r *ReadabilityExtractor) extractMetadata(doc *goquery.Document, article *Article) {
	doc.Find("meta[property^='og:']").Each(func(i int, s *goquery.Selection) {
		prop, _ := s.Attr("property")
		content, _ := s.Attr("content")

		switch prop {
		case "og:title":
			if article.Title == "" {
				article.Title = content
			}
		case "og:site_name":
			article.SiteName = content
		case "og:description":
			article.Excerpt = content
		}
	})

	doc.Find("meta[name^='twitter:']").Each(func(i int, s *goquery.Selection) {
		name, _ := s.Attr("name")
		content, _ := s.Attr("content")

		switch name {
		case "twitter:title":
			if article.Title == "" {
				article.Title = content
			}
		case "twitter:description":
			if article.Excerpt == "" {
				article.Excerpt = content
			}
		}
	})

	doc.Find("meta[name='author']").Each(func(i int, s *goquery.Selection) {
		article.Author, _ = s.Attr("content")
	})

	doc.Find("meta[name='description']").Each(func(i int, s *goquery.Selection) {
		if article.Excerpt == "" {
			article.Excerpt, _ = s.Attr("content")
		}
	})
}

func (r *ReadabilityExtractor) prepDocument(doc *goquery.Document) {
	doc.Find("script, style, nav, header, footer, aside, form, iframe, noscript").Remove()

	doc.Find("[hidden], .hidden, [style*='display:none'], [style*='display: none']").Remove()

	doc.Find(".comments, .comment, .share, .social, .sidebar, .ad, .ads, .advertisement").Remove()
}

func (r *ReadabilityExtractor) extractContent(doc *goquery.Document) string {
	candidates := r.getCandidateElements(doc)

	var bestElement *goquery.Selection
	bestScore := float64(0)

	for _, candidate := range candidates {
		if candidate.score > bestScore {
			bestScore = candidate.score
			bestElement = candidate.element
		}
	}

	if bestElement == nil {
		bestElement = doc.Find("body").First()
	}

	if bestElement == nil {
		return ""
	}

	html, err := bestElement.Html()
	if err != nil {
		return bestElement.Text()
	}

	return html
}

type candidate struct {
	element *goquery.Selection
	score   float64
}

func (r *ReadabilityExtractor) getCandidateElements(doc *goquery.Document) []candidate {
	var candidates []candidate

	doc.Find("p, div, article, section").Each(func(i int, s *goquery.Selection) {
		text := s.Text()
		text = strings.TrimSpace(text)

		if utf8.RuneCountInString(text) < r.minContentLength {
			return
		}

		score := r.scoreElement(s, text)

		if score >= r.minScore {
			candidates = append(candidates, candidate{
				element: s,
				score:   score,
			})
		}
	})

	return candidates
}

func (r *ReadabilityExtractor) scoreElement(s *goquery.Selection, text string) float64 {
	score := float64(utf8.RuneCountInString(text))

	tagName := goquery.NodeName(s)
	switch tagName {
	case "article":
		score *= 2
	case "section":
		score *= 1.5
	case "main":
		score *= 1.8
	}

	id, _ := s.Attr("id")
	class, _ := s.Attr("class")

	positivePatterns := []string{"content", "article", "post", "entry", "text", "main", "body"}
	negativePatterns := []string{"sidebar", "comment", "footer", "header", "nav", "menu", "ad", "social"}

	for _, pattern := range positivePatterns {
		if strings.Contains(strings.ToLower(id), pattern) || strings.Contains(strings.ToLower(class), pattern) {
			score *= 1.5
		}
	}

	for _, pattern := range negativePatterns {
		if strings.Contains(strings.ToLower(id), pattern) || strings.Contains(strings.ToLower(class), pattern) {
			score *= 0.5
		}
	}

	links := s.Find("a").Length()
	if links > 0 {
		linkDensity := float64(links) / float64(utf8.RuneCountInString(text)+1)
		if linkDensity > 0.3 {
			score *= 0.5
		}
	}

	return score
}

func (r *ReadabilityExtractor) htmlToText(html string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return html
	}

	return strings.TrimSpace(doc.Text())
}

func (r *ReadabilityExtractor) truncate(text string, maxLen int) string {
	text = strings.TrimSpace(text)
	if utf8.RuneCountInString(text) <= maxLen {
		return text
	}

	runed := []rune(text)
	for i := maxLen; i > maxLen-50 && i > 0; i-- {
		if runed[i] == ' ' || runed[i] == '.' || runed[i] == ',' {
			return string(runed[:i]) + "..."
		}
	}

	return string(runed[:maxLen]) + "..."
}

func (r *ReadabilityExtractor) Clean(html string) string {
	re := regexp.MustCompile(`<script[^>]*>[\s\S]*?</script>`)
	html = re.ReplaceAllString(html, "")

	re = regexp.MustCompile(`<style[^>]*>[\s\S]*?</style>`)
	html = re.ReplaceAllString(html, "")

	re = regexp.MustCompile(`<!--[\s\S]*?-->`)
	html = re.ReplaceAllString(html, "")

	return html
}
