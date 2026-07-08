package gdrive

import (
	"context"
	"time"
)

type File struct {
	ID           string
	Name         string
	MimeType     string
	ModifiedTime time.Time
	Version      int64
	LastModifier string
	Trashed      bool
}

type Permission struct {
	Type  string
	Email string
}

type API interface {
	StartPageToken(ctx context.Context) (string, error)
	Changes(ctx context.Context, pageToken string) (files []File, nextPageToken, newStartToken string, err error)
	ListFiles(ctx context.Context, from, to time.Time) ([]File, error)
	ExportText(ctx context.Context, file File) (string, error)
	Permissions(ctx context.Context, fileID string) ([]Permission, error)
	About(ctx context.Context) error
}

type Envelope struct {
	File    File   `json:"file"`
	Content string `json:"content"`
}
