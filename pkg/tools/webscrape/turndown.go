package webscrape

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

type TurndownConverter struct {
	headingStyle   string
	bulletMarker   string
	codeBlockStyle string
	fence          string
}

func NewTurndownConverter() *TurndownConverter {
	return &TurndownConverter{
		headingStyle:   "atx",
		bulletMarker:   "-",
		codeBlockStyle: "fenced",
		fence:          "```",
	}
}

func (c *TurndownConverter) Convert(htmlStr string) (string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlStr))
	if err != nil {
		return "", err
	}

	var md strings.Builder
	c.processNode(doc.Find("body").Contents(), &md, 0)

	return strings.TrimSpace(md.String()), nil
}

func (c *TurndownConverter) processNode(selection *goquery.Selection, md *strings.Builder, depth int) {
	selection.Each(func(i int, s *goquery.Selection) {
		if len(s.Nodes) == 0 {
			return
		}
		node := s.Nodes[0]

		switch node.Type {
		case html.TextNode:
			text := strings.TrimSpace(node.Data)
			if text != "" {
				md.WriteString(text)
			}

		case html.ElementNode:
			c.processElement(s, md, depth)
		}
	})
}

func (c *TurndownConverter) processElement(s *goquery.Selection, md *strings.Builder, depth int) {
	tagName := goquery.NodeName(s)

	switch tagName {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		c.processHeading(s, md, tagName)

	case "p":
		md.WriteString("\n\n")
		c.processNode(s.Contents(), md, depth)
		md.WriteString("\n\n")

	case "br":
		md.WriteString("  \n")

	case "strong", "b":
		md.WriteString("**")
		c.processNode(s.Contents(), md, depth)
		md.WriteString("**")

	case "em", "i":
		md.WriteString("*")
		c.processNode(s.Contents(), md, depth)
		md.WriteString("*")

	case "code":
		if len(s.Parent().Nodes) > 0 && s.Parent().Nodes[0].Data == "pre" {
		} else {
			md.WriteString("`")
			md.WriteString(s.Text())
			md.WriteString("`")
		}

	case "pre":
		md.WriteString("\n\n")
		lang, _ := s.Attr("class")
		if lang != "" {
			re := regexp.MustCompile(`language-(\w+)`)
			match := re.FindStringSubmatch(lang)
			if len(match) > 1 {
				md.WriteString(c.fence + match[1] + "\n")
			} else {
				md.WriteString(c.fence + "\n")
			}
		} else {
			md.WriteString(c.fence + "\n")
		}
		md.WriteString(s.Text())
		md.WriteString("\n" + c.fence + "\n\n")

	case "blockquote":
		md.WriteString("\n\n")
		lines := strings.Split(s.Text(), "\n")
		for _, line := range lines {
			md.WriteString("> " + line + "\n")
		}
		md.WriteString("\n")

	case "ul", "ol":
		md.WriteString("\n\n")
		c.processList(s, md, depth, tagName == "ol")
		md.WriteString("\n")

	case "li":

	case "a":
		href, _ := s.Attr("href")
		text := s.Text()
		if href != "" && text != "" {
			md.WriteString("[" + text + "](" + href + ")")
		} else {
			c.processNode(s.Contents(), md, depth)
		}

	case "img":
		src, _ := s.Attr("src")
		alt, _ := s.Attr("alt")
		if src != "" {
			md.WriteString("![" + alt + "](" + src + ")")
		}

	case "hr":
		md.WriteString("\n\n---\n\n")

	case "div", "span", "section", "article", "main":
		c.processNode(s.Contents(), md, depth)

	case "table":
		c.processTable(s, md)

	default:
		c.processNode(s.Contents(), md, depth)
	}
}

func (c *TurndownConverter) processHeading(s *goquery.Selection, md *strings.Builder, tag string) {
	level := int(tag[1] - '0')

	if c.headingStyle == "atx" {
		md.WriteString(strings.Repeat("#", level) + " ")
	}

	text := strings.TrimSpace(s.Text())
	md.WriteString(text)

	if c.headingStyle == "setext" && (level == 1 || level == 2) {
		md.WriteString("\n")
		if level == 1 {
			md.WriteString(strings.Repeat("=", utf8.RuneCountInString(text)))
		} else {
			md.WriteString(strings.Repeat("-", utf8.RuneCountInString(text)))
		}
	}

	md.WriteString("\n\n")
}

func (c *TurndownConverter) processList(s *goquery.Selection, md *strings.Builder, depth int, ordered bool) {
	counter := 1
	indent := strings.Repeat("    ", depth)

	s.Children().Each(func(i int, li *goquery.Selection) {
		if goquery.NodeName(li) != "li" {
			return
		}

		md.WriteString(indent)
		if ordered {
			md.WriteString(fmt.Sprintf("%d. ", counter))
			counter++
		} else {
			md.WriteString(c.bulletMarker + " ")
		}

		c.processNode(li.Contents(), md, depth+1)
		md.WriteString("\n")
	})
}

func (c *TurndownConverter) processTable(s *goquery.Selection, md *strings.Builder) {
	md.WriteString("\n\n")

	headers := s.Find("th")
	if headers.Length() > 0 {
		md.WriteString("|")
		headers.Each(func(i int, th *goquery.Selection) {
			md.WriteString(" " + th.Text() + " |")
		})
		md.WriteString("\n|")
		headers.Each(func(i int, th *goquery.Selection) {
			md.WriteString(" --- |")
		})
		md.WriteString("\n")
	}

	s.Find("tr").Each(func(i int, tr *goquery.Selection) {
		if tr.Find("th").Length() > 0 {
			return
		}

		md.WriteString("|")
		tr.Find("td").Each(func(j int, td *goquery.Selection) {
			md.WriteString(" " + td.Text() + " |")
		})
		md.WriteString("\n")
	})

	md.WriteString("\n")
}

func (c *TurndownConverter) ConvertWithFrontmatter(html string, article *Article) (string, error) {
	md, err := c.Convert(html)
	if err != nil {
		return "", err
	}

	var result strings.Builder

	if article.Title != "" {
		result.WriteString("# " + article.Title + "\n\n")
	}

	var meta []string
	if article.Author != "" {
		meta = append(meta, "By "+article.Author)
	}
	if article.SiteName != "" {
		meta = append(meta, article.SiteName)
	}
	if len(meta) > 0 {
		result.WriteString("*" + strings.Join(meta, " • ") + "*\n\n")
	}

	result.WriteString(md)

	return result.String(), nil
}
