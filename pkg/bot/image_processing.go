package bot

import (
	"fmt"
	"log"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// Supported image MIME types for vision processing
var supportedImageTypes = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/jpg":  true,
	"image/webp": true,
	"image/gif":  true,
}

// processImageAttachments checks for image attachments and returns a description
// Returns empty string if no images or vision client not configured
func (h *Handler) processImageAttachments(attachments []*discordgo.MessageAttachment) string {
	if h.visionClient == nil {
		return ""
	}

	if len(attachments) == 0 {
		return ""
	}

	var imageDescriptions []string

	for _, attachment := range attachments {
		// Check if it's an image by content type
		if attachment.ContentType == "" {
			// Try to guess from filename
			lower := strings.ToLower(attachment.Filename)
			if !strings.HasSuffix(lower, ".png") &&
				!strings.HasSuffix(lower, ".jpg") &&
				!strings.HasSuffix(lower, ".jpeg") &&
				!strings.HasSuffix(lower, ".webp") {
				continue
			}
		} else if !supportedImageTypes[attachment.ContentType] {
			continue
		}

		// Check file size (limit to ~7MB for Gemini inline)
		if attachment.Size > 7*1024*1024 {
			log.Printf("Skipping image %s: too large (%d bytes)", attachment.Filename, attachment.Size)
			continue
		}

		log.Printf("Processing image: %s (%s, %d bytes)", attachment.Filename, attachment.ContentType, attachment.Size)

		// Get image description from vision client
		result, err := h.visionClient.DescribeImageFromURL(attachment.URL)
		if err != nil {
			log.Printf("Error processing image %s: %v", attachment.Filename, err)
			continue
		}

		if result.IsNSFW {
			// Still acknowledge the image but note it was flagged
			imageDescriptions = append(imageDescriptions, "[User sent an image that appears to be NSFW/spicy content]")
			continue
		}

		if result.Error != nil {
			log.Printf("Vision API error for %s: %v", attachment.Filename, result.Error)
			continue
		}

		if result.Description != "" {
			imageDescriptions = append(imageDescriptions, result.Description)
		}
	}

	if len(imageDescriptions) == 0 {
		return ""
	}

	// Format descriptions for the LLM context
	if len(imageDescriptions) == 1 {
		return fmt.Sprintf("[User sent an image: %s]", imageDescriptions[0])
	}

	return fmt.Sprintf("[User sent %d images: %s]", len(imageDescriptions), strings.Join(imageDescriptions, " | "))
}
