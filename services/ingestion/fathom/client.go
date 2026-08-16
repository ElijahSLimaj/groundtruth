package fathom

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"golang.org/x/time/rate"
)

const defaultBaseURL = "https://api.fathom.ai"

type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	limiter    *rate.Limiter
}

func NewClient(apiKey string) *Client {
	return &Client{
		apiKey:     apiKey,
		baseURL:    defaultBaseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		limiter:    rate.NewLimiter(rate.Every(time.Second/5), 5),
	}
}

type wireList struct {
	NextCursor *string       `json:"next_cursor"`
	Items      []wireMeeting `json:"items"`
}

type wireMeeting struct {
	RecordingID        int64   `json:"recording_id"`
	Title              string  `json:"title"`
	MeetingType        *string `json:"meeting_type"`
	ShareURL           string  `json:"share_url"`
	RecordingStartTime string  `json:"recording_start_time"`
	CalendarInvitees   []struct {
		Email string `json:"email"`
	} `json:"calendar_invitees"`
	RecordedBy struct {
		Email string `json:"email"`
	} `json:"recorded_by"`
	Transcript []struct {
		Speaker struct {
			DisplayName string `json:"display_name"`
		} `json:"speaker"`
		Text      string `json:"text"`
		Timestamp string `json:"timestamp"`
	} `json:"transcript"`
}

func (m wireMeeting) decode() (Meeting, error) {
	start, err := time.Parse(time.RFC3339, m.RecordingStartTime)
	if err != nil {
		return Meeting{}, fmt.Errorf("meeting %d start time %q: %w", m.RecordingID, m.RecordingStartTime, err)
	}
	invitees := make([]string, 0, len(m.CalendarInvitees))
	for _, invitee := range m.CalendarInvitees {
		if invitee.Email != "" {
			invitees = append(invitees, invitee.Email)
		}
	}
	segments := make([]Segment, 0, len(m.Transcript))
	for _, s := range m.Transcript {
		segments = append(segments, Segment{
			Speaker:   s.Speaker.DisplayName,
			Text:      s.Text,
			Timestamp: s.Timestamp,
		})
	}
	meetingType := ""
	if m.MeetingType != nil {
		meetingType = *m.MeetingType
	}
	return Meeting{
		RecordingID: m.RecordingID,
		Title:       m.Title,
		MeetingType: meetingType,
		ShareURL:    m.ShareURL,
		StartTime:   start.UTC(),
		Invitees:    invitees,
		RecordedBy:  m.RecordedBy.Email,
		Transcript:  segments,
	}, nil
}

func (c *Client) List(ctx context.Context, cursor string) (Page, error) {
	params := url.Values{"include_transcript": {"true"}}
	if cursor != "" {
		params.Set("cursor", cursor)
	}
	if err := c.limiter.Wait(ctx); err != nil {
		return Page{}, err
	}
	target := c.baseURL + "/external/v1/meetings?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return Page{}, fmt.Errorf("fathom request: %w", err)
	}
	req.Header.Set("X-Api-Key", c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Page{}, fmt.Errorf("fathom meetings: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return Page{}, fmt.Errorf("fathom meetings: read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return Page{}, fmt.Errorf("fathom meetings: status %d: %s", resp.StatusCode, truncate(string(data), 200))
	}
	var out wireList
	if err := json.Unmarshal(data, &out); err != nil {
		return Page{}, fmt.Errorf("fathom meetings: decode: %w", err)
	}
	meetings := make([]Meeting, 0, len(out.Items))
	for _, raw := range out.Items {
		meeting, err := raw.decode()
		if err != nil {
			return Page{}, err
		}
		meetings = append(meetings, meeting)
	}
	next := ""
	if out.NextCursor != nil {
		next = *out.NextCursor
	}
	return Page{Meetings: meetings, NextCursor: next}, nil
}

func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[:limit]
}

func recordingKey(id int64) string {
	return strconv.FormatInt(id, 10)
}
