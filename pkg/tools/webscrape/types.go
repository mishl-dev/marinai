package webscrape

import "time"

type ScrapedContent struct {
	URL         string    `json:"url"`
	Title       string    `json:"title"`
	Content     string    `json:"content"`
	Author      string    `json:"author,omitempty"`
	PublishedAt time.Time `json:"published_at,omitempty"`
	SiteName    string    `json:"site_name,omitempty"`
	Excerpt     string    `json:"excerpt,omitempty"`
}

type FetchedPage struct {
	URL         string
	StatusCode  int
	ContentType string
	HTML        string
	Headers     map[string]string
}

type YouTubeTranscript struct {
	VideoID  string              `json:"video_id"`
	Title    string              `json:"title"`
	Segments []TranscriptSegment `json:"segments"`
	Duration int                 `json:"duration_seconds"`
}

type TranscriptSegment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

type ScrapeOptions struct {
	Timeout      time.Duration
	MaxBodySize  int64
	ExtractMain  bool
	OutputFormat string
}
