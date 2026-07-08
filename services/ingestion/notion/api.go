package notion

import (
	"context"
	"time"
)

type Page struct {
	ID             string
	Title          string
	URL            string
	LastEditedTime time.Time
	LastEditedBy   string
}

type API interface {
	SearchPages(ctx context.Context, startCursor string) (pages []Page, nextCursor string, err error)
	PageText(ctx context.Context, pageID string) (string, error)
	Me(ctx context.Context) error
}

type Envelope struct {
	Page    Page   `json:"page"`
	Content string `json:"content"`
}
