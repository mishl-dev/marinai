package tools

import (
	"net/http"
	"strings"
)

type ContentType string

const (
	ContentTypeHTML    ContentType = "html"
	ContentTypeText    ContentType = "text"
	ContentTypePDF     ContentType = "pdf"
	ContentTypeImage   ContentType = "image"
	ContentTypeAudio   ContentType = "audio"
	ContentTypeVideo   ContentType = "video"
	ContentTypeBinary  ContentType = "binary"
	ContentTypeUnknown ContentType = "unknown"
)

func DetectContentType(data []byte, mimeType string) ContentType {
	if len(data) > 0 {
		switch {
		case isPDF(data):
			return ContentTypePDF
		case isImage(data):
			return ContentTypeImage
		case isAudio(data):
			return ContentTypeAudio
		case isVideo(data):
			return ContentTypeVideo
		case isHTML(data):
			return ContentTypeHTML
		}
	}

	return DetectFromMIME(mimeType)
}

func DetectFromMIME(mimeType string) ContentType {
	mimeType = strings.ToLower(mimeType)

	switch {
	case strings.Contains(mimeType, "text/html"):
		return ContentTypeHTML
	case strings.HasPrefix(mimeType, "text/"):
		return ContentTypeText
	case strings.Contains(mimeType, "application/pdf"):
		return ContentTypePDF
	case strings.HasPrefix(mimeType, "image/"):
		return ContentTypeImage
	case strings.HasPrefix(mimeType, "audio/"):
		return ContentTypeAudio
	case strings.HasPrefix(mimeType, "video/"):
		return ContentTypeVideo
	default:
		return ContentTypeUnknown
	}
}

func isPDF(data []byte) bool {
	return len(data) > 4 && string(data[0:4]) == "%PDF"
}

func isImage(data []byte) bool {
	if len(data) < 4 {
		return false
	}

	if data[0] == 0x89 && string(data[1:4]) == "PNG" {
		return true
	}

	if data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return true
	}

	if len(data) > 6 && (string(data[0:6]) == "GIF87a" || string(data[0:6]) == "GIF89a") {
		return true
	}

	if len(data) > 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return true
	}

	return false
}

func isAudio(data []byte) bool {
	if len(data) < 4 {
		return false
	}

	if string(data[0:3]) == "ID3" || (data[0] == 0xFF && (data[1]&0xE0) == 0xE0) {
		return true
	}

	if len(data) > 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WAVE" {
		return true
	}

	if len(data) > 12 && string(data[4:8]) == "ftyp" {
		return true
	}

	if string(data[0:4]) == "OggS" {
		return true
	}

	if string(data[0:4]) == "fLaC" {
		return true
	}

	return false
}

func isVideo(data []byte) bool {
	if len(data) < 12 {
		return false
	}

	if string(data[4:8]) == "ftyp" {
		return true
	}

	if string(data[0:4]) == "\x1a\x45\xdf\xa3" {
		return true
	}

	if string(data[0:4]) == "RIFF" && string(data[8:11]) == "AVI" {
		return true
	}

	return false
}

func isHTML(data []byte) bool {
	if len(data) < 20 {
		return false
	}

	lower := strings.ToLower(string(data[:min(200, len(data))]))

	return strings.Contains(lower, "<!doctype html") ||
		strings.Contains(lower, "<html") ||
		strings.Contains(lower, "<head") ||
		strings.Contains(lower, "<body")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func GetImageFormat(data []byte) string {
	if len(data) < 4 {
		return ""
	}

	switch {
	case data[0] == 0x89 && string(data[1:4]) == "PNG":
		return "png"
	case data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF:
		return "jpeg"
	case len(data) > 6 && (string(data[0:6]) == "GIF87a" || string(data[0:6]) == "GIF89a"):
		return "gif"
	case len(data) > 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		return "webp"
	default:
		return ""
	}
}

func GetAudioFormat(data []byte) string {
	if len(data) < 4 {
		return ""
	}

	switch {
	case string(data[0:3]) == "ID3" || (data[0] == 0xFF && (data[1]&0xE0) == 0xE0):
		return "mp3"
	case len(data) > 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WAVE":
		return "wav"
	case len(data) > 12 && string(data[4:8]) == "ftyp":
		return "m4a"
	case string(data[0:4]) == "OggS":
		return "ogg"
	case string(data[0:4]) == "fLaC":
		return "flac"
	default:
		return ""
	}
}

func DetectFromHTTPResponse(resp *http.Response, body []byte) ContentType {
	mimeType := resp.Header.Get("Content-Type")
	return DetectContentType(body, mimeType)
}
