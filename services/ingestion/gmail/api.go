package gmail

import (
	"context"
	"time"
)

type Message struct {
	ID           string
	ThreadID     string
	InternalDate time.Time
	From         string
	To           []string
	Cc           []string
	Subject      string
	LabelIDs     []string
	Body         string
}

type API interface {
	Profile(ctx context.Context) (email string, historyID uint64, err error)
	ListHistory(ctx context.Context, startHistoryID uint64) (messageIDs []string, newHistoryID uint64, err error)
	ListMessages(ctx context.Context, query string) (messageIDs []string, err error)
	GetMessage(ctx context.Context, id string) (Message, error)
}
