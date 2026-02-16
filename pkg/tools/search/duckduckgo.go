package search

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type DuckDuckGoClient struct {
	client     *http.Client
	userAgents []string
}

func NewDuckDuckGoClient() *DuckDuckGoClient {
	return &DuckDuckGoClient{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		userAgents: []string{
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:121.0) Gecko/20100101 Firefox/121.0",
			"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Safari/605.1.15",
		},
	}
}

func (c *DuckDuckGoClient) Search(ctx context.Context, query string, opts SearchOptions) ([]SearchResult, error) {
	if opts.MaxResults <= 0 {
		opts.MaxResults = 10
	}

	searchURL := c.buildSearchURL(query, opts)

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	c.setHeaders(req)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	results := c.parseHTMLResults(string(body), opts.MaxResults)

	return results, nil
}

func (c *DuckDuckGoClient) buildSearchURL(query string, opts SearchOptions) string {
	params := url.Values{}
	params.Set("q", query)
	params.Set("kl", opts.Region)

	if opts.SafeSearch == "off" {
		params.Set("kp", "-2")
	} else if opts.SafeSearch == "on" {
		params.Set("kp", "1")
	}

	if opts.TimeLimit != "" {
		params.Set("df", opts.TimeLimit)
	}

	return "https://html.duckduckgo.com/html/?" + params.Encode()
}

func (c *DuckDuckGoClient) setHeaders(req *http.Request) {
	req.Header.Set("User-Agent", c.userAgents[rand.Intn(len(c.userAgents))])
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	req.Header.Set("Accept-Encoding", "gzip, deflate")
	req.Header.Set("DNT", "1")
	req.Header.Set("Connection", "keep-alive")
}

func (c *DuckDuckGoClient) parseHTMLResults(html string, maxResults int) []SearchResult {
	var results []SearchResult

	linkRegex := regexp.MustCompile(`<a[^>]*class="result__a"[^>]*href="([^"]+)"[^>]*>([^<]+)</a>`)
	snippetRegex := regexp.MustCompile(`<a[^>]*class="result__snippet"[^>]*>([^<]*(?:<[^>]+>[^<]*)*)</a>`)

	links := linkRegex.FindAllStringSubmatch(html, -1)
	snippets := snippetRegex.FindAllStringSubmatch(html, -1)

	count := min(len(links), maxResults)

	for i := 0; i < count; i++ {
		result := SearchResult{}

		rawURL := links[i][1]
		result.URL = c.extractActualURL(rawURL)

		result.Title = strings.TrimSpace(c.stripHTML(links[i][2]))

		if i < len(snippets) {
			result.Body = strings.TrimSpace(c.stripHTML(snippets[i][1]))
		}

		if result.URL != "" && result.Title != "" {
			results = append(results, result)
		}
	}

	return results
}

func (c *DuckDuckGoClient) extractActualURL(ddgURL string) string {
	if strings.Contains(ddgURL, "uddg=") {
		uddgStart := strings.Index(ddgURL, "uddg=")
		if uddgStart == -1 {
			return ddgURL
		}

		uddgStart += 5
		uddgEnd := strings.Index(ddgURL[uddgStart:], "&")
		if uddgEnd == -1 {
			uddgEnd = len(ddgURL) - uddgStart
		}

		encoded := ddgURL[uddgStart : uddgStart+uddgEnd]
		decoded, err := url.QueryUnescape(encoded)
		if err != nil {
			return ddgURL
		}
		return decoded
	}

	if strings.HasPrefix(ddgURL, "//") {
		return "https:" + ddgURL
	}

	return ddgURL
}

func (c *DuckDuckGoClient) stripHTML(input string) string {
	re := regexp.MustCompile(`<[^>]+>`)
	result := re.ReplaceAllString(input, "")

	result = strings.ReplaceAll(result, "&amp;", "&")
	result = strings.ReplaceAll(result, "&lt;", "<")
	result = strings.ReplaceAll(result, "&gt;", ">")
	result = strings.ReplaceAll(result, "&quot;", "\"")
	result = strings.ReplaceAll(result, "&#39;", "'")
	result = strings.ReplaceAll(result, "&nbsp;", " ")

	return result
}

func (c *DuckDuckGoClient) SearchInstant(ctx context.Context, query string) (map[string]interface{}, error) {
	params := url.Values{}
	params.Set("q", query)
	params.Set("format", "json")
	params.Set("no_html", "1")
	params.Set("skip_disambig", "1")

	apiURL := "https://api.duckduckgo.com/?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", c.userAgents[rand.Intn(len(c.userAgents))])

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	return result, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
