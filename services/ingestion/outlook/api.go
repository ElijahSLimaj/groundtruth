package outlook

import (
	"context"
	"time"
)

type Message struct {
	ID               string
	ThreadID         string
	ReceivedDateTime time.Time
	From             string
	To               []string
	Cc               []string
	Subject          string
	Categories       []string
	Body             string
}

type API interface {
	Profile(ctx context.Context) (email string, err error)
	FolderDelta(ctx context.Context, folderID, deltaLink string) (messageIDs []string, nextDeltaLink string, err error)
	ListMessages(ctx context.Context, query string) (messageIDs []string, err error)
	GetMessage(ctx context.Context, id string) (Message, error)
}
