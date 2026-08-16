package hubspot

import (
	"context"
	"time"
)

type Record struct {
	ID         string
	Properties map[string]string
	UpdatedAt  time.Time
}

type Page struct {
	Records   []Record
	NextAfter string
}

type API interface {
	Search(ctx context.Context, gteMillis, lteMillis, after string) (Page, error)
	Ping(ctx context.Context) error
}
