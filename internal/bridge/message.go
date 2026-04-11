package bridge

import (
	"fmt"
	"regexp"

	"github.com/a2aproject/a2a-go/v2/a2a"
)

// documentIDPattern matches document IDs like DOC-001, DOC-002, etc.
var documentIDPattern = regexp.MustCompile(`\b(DOC-\d+)\b`)

// ExtractDocumentID extracts a document ID from an A2A message.
// It first checks for a DataPart with a "document_id" field, then falls back
// to extracting a DOC-NNN pattern from TextPart content.
func ExtractDocumentID(msg *a2a.Message) (string, error) {
	if msg == nil {
		return "", fmt.Errorf("message is nil")
	}

	// First pass: look for structured DataPart with document_id
	for _, part := range msg.Parts {
		data := part.Data()
		if data == nil {
			continue
		}
		if m, ok := data.(map[string]any); ok {
			if v, ok := m["document_id"].(string); ok && v != "" {
				return v, nil
			}
		}
	}

	// Second pass: extract document ID from text (e.g., "Summarize DOC-002")
	for _, part := range msg.Parts {
		text := part.Text()
		if text == "" {
			continue
		}
		if matches := documentIDPattern.FindStringSubmatch(text); len(matches) > 1 {
			return matches[1], nil
		}
	}

	return "", fmt.Errorf("no document ID found in message (use DataPart with document_id or include DOC-NNN in text)")
}

// ExtractReviewType extracts an optional review type from an A2A message DataPart.
func ExtractReviewType(msg *a2a.Message) string {
	if msg == nil {
		return ""
	}
	for _, part := range msg.Parts {
		data := part.Data()
		if data == nil {
			continue
		}
		if m, ok := data.(map[string]any); ok {
			if v, ok := m["review_type"].(string); ok {
				return v
			}
		}
	}
	return ""
}
