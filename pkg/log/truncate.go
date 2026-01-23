package log

import "fmt"

const (
	// DefaultMaxBodySize is the default maximum size for body content before truncation
	DefaultMaxBodySize = 2048
	// DefaultPreviewSize is the default size of the preview shown for truncated content
	DefaultPreviewSize = 500
)

// TruncateBody truncates a string body if it exceeds maxSize.
// Returns original string if within limit, otherwise returns truncated format:
// [LARGE_BODY: truncated, size: %d bytes, preview: %s...]
func TruncateBody(body string, maxSize, previewSize int) string {
	bodyLen := len(body)
	if bodyLen <= maxSize {
		return body
	}

	if previewSize > bodyLen {
		previewSize = bodyLen
	} else if previewSize > maxSize {
		previewSize = maxSize
	}

	return fmt.Sprintf("[LARGE_BODY: truncated, size: %d bytes, preview: %s...]",
		bodyLen, body[:previewSize])
}

// TruncateBodyDefault truncates a string body using default size limits.
func TruncateBodyDefault(body string) string {
	return TruncateBody(body, DefaultMaxBodySize, DefaultPreviewSize)
}
