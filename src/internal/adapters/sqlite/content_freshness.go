package sqlite

import (
	"context"
	"errors"
	"strings"

	"tangentry/internal/content"
	"tangentry/internal/freshness"
)

// CurrentContentVersion returns the live saved_content revision and hash for
// Owner E freshness checks. A missing row is Exists=false with a nil error.
func (s *Store) CurrentContentVersion(ctx context.Context, id string) (freshness.Current, error) {
	item, err := s.Get(ctx, id)
	if errors.Is(err, content.ErrNotFound) {
		return freshness.Current{Exists: false}, nil
	}
	if err != nil {
		return freshness.Current{}, err
	}
	return freshness.Current{Exists: true, Revision: item.ContentRevision, Hash: item.ContentHash}, nil
}

// SavedSourceText returns the canonical saved text and the revision/hash of
// that same row. Chat uses it to check client-supplied context. The text
// follows the reader order used for article and description context: article
// blocks, otherwise discussion block text, otherwise the description.
func (s *Store) SavedSourceText(ctx context.Context, id string) (string, freshness.Current, error) {
	item, err := s.Get(ctx, id)
	if errors.Is(err, content.ErrNotFound) {
		return "", freshness.Current{Exists: false}, nil
	}
	if err != nil {
		return "", freshness.Current{}, err
	}
	return canonicalSourceText(item), freshness.Current{Exists: true, Revision: item.ContentRevision, Hash: item.ContentHash}, nil
}

func canonicalSourceText(item content.Item) string {
	if len(item.ArticleBlocks) > 0 {
		parts := make([]string, 0, len(item.ArticleBlocks))
		for _, block := range item.ArticleBlocks {
			text := strings.TrimSpace(block.Text)
			if alt := strings.TrimSpace(block.Alt); alt != "" {
				if text != "" {
					text += "\n" + alt
				} else {
					text = alt
				}
			}
			if text != "" {
				parts = append(parts, text)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n")
		}
	}
	if item.Discussion != nil {
		parts := make([]string, 0)
		for _, post := range item.Discussion.Posts {
			for _, block := range post.Blocks {
				if text := strings.TrimSpace(block.Text); text != "" {
					parts = append(parts, text)
				}
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n")
		}
	}
	if text := strings.TrimSpace(item.Description); text != "" {
		return text
	}
	return strings.TrimSpace(item.PreviewText)
}
