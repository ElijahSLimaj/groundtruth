package fathom

import (
	"context"
	"time"
)

type Segment struct {
	Speaker   string
	Text      string
	Timestamp string
}

type Meeting struct {
	RecordingID int64
	Title       string
	MeetingType string
	ShareURL    string
	StartTime   time.Time
	Invitees    []string
	RecordedBy  string
	Transcript  []Segment
}

type Page struct {
	Meetings   []Meeting
	NextCursor string
}

type API interface {
	List(ctx context.Context, cursor string) (Page, error)
}
