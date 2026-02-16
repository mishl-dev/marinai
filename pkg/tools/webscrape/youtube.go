package webscrape

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type YouTubeExtractor struct {
	client *http.Client
}

func NewYouTubeExtractor() *YouTubeExtractor {
	return &YouTubeExtractor{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (y *YouTubeExtractor) GetVideoID(youtubeURL string) (string, error) {
	parsed, err := url.Parse(youtubeURL)
	if err != nil {
		return "", err
	}

	var videoID string

	if strings.Contains(parsed.Host, "youtube.com") {
		query := parsed.Query()
		videoID = query.Get("v")
	}

	if strings.Contains(parsed.Host, "youtu.be") {
		videoID = strings.TrimPrefix(parsed.Path, "/")
	}

	if strings.Contains(parsed.Path, "/embed/") {
		videoID = strings.TrimPrefix(parsed.Path, "/embed/")
	}

	if strings.Contains(parsed.Path, "/shorts/") {
		videoID = strings.TrimPrefix(parsed.Path, "/shorts/")
	}

	if videoID == "" {
		return "", fmt.Errorf("could not extract video ID from URL: %s", youtubeURL)
	}

	videoID = strings.Split(videoID, "?")[0]
	videoID = strings.Split(videoID, "/")[0]

	return videoID, nil
}

func (y *YouTubeExtractor) GetTranscript(ctx context.Context, videoID string) (*YouTubeTranscript, error) {
	videoURL := fmt.Sprintf("https://www.youtube.com/watch?v=%s", videoID)

	req, err := http.NewRequestWithContext(ctx, "GET", videoURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := y.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	html := string(body)

	captionTracks, err := y.extractCaptionTracks(html)
	if err != nil {
		return nil, fmt.Errorf("extract caption tracks: %w", err)
	}

	if len(captionTracks) == 0 {
		return nil, fmt.Errorf("no captions available for video %s", videoID)
	}

	var track captionTrack
	for _, t := range captionTracks {
		if strings.Contains(t.LanguageCode, "en") {
			track = t
			break
		}
	}

	if track.URL == "" && len(captionTracks) > 0 {
		track = captionTracks[0]
	}

	segments, err := y.fetchCaptions(ctx, track.URL)
	if err != nil {
		return nil, fmt.Errorf("fetch captions: %w", err)
	}

	title := y.extractVideoTitle(html)

	return &YouTubeTranscript{
		VideoID:  videoID,
		Title:    title,
		Segments: segments,
	}, nil
}

type captionTrack struct {
	URL          string `json:"baseUrl"`
	LanguageCode string `json:"languageCode"`
	Name         string `json:"name"`
}

func (y *YouTubeExtractor) extractCaptionTracks(html string) ([]captionTrack, error) {
	re := regexp.MustCompile(`ytInitialPlayerResponse\s*=\s*({.+?});`)
	match := re.FindStringSubmatch(html)

	if len(match) < 2 {
		return nil, fmt.Errorf("could not find ytInitialPlayerResponse")
	}

	var playerResponse struct {
		Captions struct {
			PlayerCaptionsTracklistRenderer struct {
				CaptionTracks []captionTrack `json:"captionTracks"`
			} `json:"playerCaptionsTracklistRenderer"`
		} `json:"captions"`
	}

	if err := json.Unmarshal([]byte(match[1]), &playerResponse); err != nil {
		return nil, err
	}

	return playerResponse.Captions.PlayerCaptionsTracklistRenderer.CaptionTracks, nil
}

func (y *YouTubeExtractor) fetchCaptions(ctx context.Context, captionURL string) ([]TranscriptSegment, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", captionURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := y.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return y.parseCaptionXML(string(body))
}

func (y *YouTubeExtractor) parseCaptionXML(xml string) ([]TranscriptSegment, error) {
	var segments []TranscriptSegment

	re := regexp.MustCompile(`<text\s+start="([\d.]+)"\s+dur="([\d.]+)"[^>]*>([^<]+)</text>`)
	matches := re.FindAllStringSubmatch(xml, -1)

	for _, match := range matches {
		var start, dur float64
		fmt.Sscanf(match[1], "%f", &start)
		fmt.Sscanf(match[2], "%f", &dur)

		text := y.decodeHTMLEntities(match[3])
		text = strings.TrimSpace(text)

		if text != "" {
			segments = append(segments, TranscriptSegment{
				Start: start,
				End:   start + dur,
				Text:  text,
			})
		}
	}

	return segments, nil
}

func (y *YouTubeExtractor) decodeHTMLEntities(s string) string {
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", "\"")
	s = strings.ReplaceAll(s, "&#39;", "'")
	s = strings.ReplaceAll(s, "&apos;", "'")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

func (y *YouTubeExtractor) extractVideoTitle(html string) string {
	re := regexp.MustCompile(`<meta\s+property="og:title"\s+content="([^"]+)"`)
	match := re.FindStringSubmatch(html)
	if len(match) > 1 {
		return match[1]
	}

	re = regexp.MustCompile(`<title>([^<]+)</title>`)
	match = re.FindStringSubmatch(html)
	if len(match) > 1 {
		title := match[1]
		title = strings.TrimSuffix(title, " - YouTube")
		return title
	}

	return ""
}

func (t *YouTubeTranscript) GetTranscriptText() string {
	var sb strings.Builder
	for _, seg := range t.Segments {
		sb.WriteString(seg.Text)
		sb.WriteString(" ")
	}
	return strings.TrimSpace(sb.String())
}

func (t *YouTubeTranscript) GetTranscriptWithTimestamps() string {
	var sb strings.Builder
	for _, seg := range t.Segments {
		startTime := formatTime(seg.Start)
		sb.WriteString(fmt.Sprintf("[%s] %s\n", startTime, seg.Text))
	}
	return sb.String()
}

func formatTime(seconds float64) string {
	mins := int(seconds) / 60
	secs := int(seconds) % 60
	return fmt.Sprintf("%d:%02d", mins, secs)
}

func IsYouTubeURL(url string) bool {
	return strings.Contains(url, "youtube.com") ||
		strings.Contains(url, "youtu.be")
}
