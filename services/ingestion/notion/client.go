package notion

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

const (
	defaultBaseURL = "https://api.notion.com/v1"
	notionVersion  = "2022-06-28"
)

type Client struct {
	token      string
	baseURL    string
	httpClient *http.Client
	limiter    *rate.Limiter
}

func NewClient(token string) *Client {
	return &Client{
		token:      token,
		baseURL:    defaultBaseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		limiter:    rate.NewLimiter(rate.Every(time.Second/3), 3),
	}
}

type wirePage struct {
	ID             string `json:"id"`
	URL            string `json:"url"`
	LastEditedTime string `json:"last_edited_time"`
	LastEditedBy   struct {
		ID string `json:"id"`
	} `json:"last_edited_by"`
	Properties map[string]struct {
		Type  string `json:"type"`
		Title []struct {
			PlainText string `json:"plain_text"`
		} `json:"title"`
	} `json:"properties"`
}

func (w wirePage) decode() (Page, error) {
	edited, err := time.Parse(time.RFC3339, w.LastEditedTime)
	if err != nil {
		return Page{}, fmt.Errorf("page %s last edited %q: %w", w.ID, w.LastEditedTime, err)
	}
	title := ""
	for _, property := range w.Properties {
		if property.Type != "title" {
			continue
		}
		parts := make([]string, 0, len(property.Title))
		for _, span := range property.Title {
			parts = append(parts, span.PlainText)
		}
		title = strings.Join(parts, "")
	}
	return Page{
		ID:             w.ID,
		Title:          title,
		URL:            w.URL,
		LastEditedTime: edited,
		LastEditedBy:   w.LastEditedBy.ID,
	}, nil
}

func (c *Client) SearchPages(ctx context.Context, startCursor string) ([]Page, string, error) {
	request := map[string]any{
		"filter":    map[string]string{"property": "object", "value": "page"},
		"sort":      map[string]string{"timestamp": "last_edited_time", "direction": "descending"},
		"page_size": 100,
	}
	if startCursor != "" {
		request["start_cursor"] = startCursor
	}
	var out struct {
		Results    []wirePage `json:"results"`
		NextCursor string     `json:"next_cursor"`
		HasMore    bool       `json:"has_more"`
	}
	if err := c.call(ctx, http.MethodPost, "/search", request, &out); err != nil {
		return nil, "", err
	}
	pages := make([]Page, 0, len(out.Results))
	for _, wire := range out.Results {
		page, err := wire.decode()
		if err != nil {
			return nil, "", err
		}
		pages = append(pages, page)
	}
	next := ""
	if out.HasMore {
		next = out.NextCursor
	}
	return pages, next, nil
}

func (c *Client) PageText(ctx context.Context, pageID string) (string, error) {
	var lines []string
	cursor := ""
	for {
		path := "/blocks/" + url.PathEscape(pageID) + "/children?page_size=100"
		if cursor != "" {
			path += "&start_cursor=" + url.QueryEscape(cursor)
		}
		var out struct {
			Results    []json.RawMessage `json:"results"`
			NextCursor string            `json:"next_cursor"`
			HasMore    bool              `json:"has_more"`
		}
		if err := c.call(ctx, http.MethodGet, path, nil, &out); err != nil {
			return "", err
		}
		for _, raw := range out.Results {
			if text := blockText(raw); text != "" {
				lines = append(lines, text)
			}
		}
		if !out.HasMore {
			break
		}
		cursor = out.NextCursor
	}
	return strings.Join(lines, "\n\n"), nil
}

func blockText(raw json.RawMessage) string {
	var block struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &block); err != nil || block.Type == "" {
		return ""
	}
	var typed map[string]json.RawMessage
	if err := json.Unmarshal(raw, &typed); err != nil {
		return ""
	}
	payload, ok := typed[block.Type]
	if !ok {
		return ""
	}
	var content struct {
		RichText []struct {
			PlainText string `json:"plain_text"`
		} `json:"rich_text"`
	}
	if err := json.Unmarshal(payload, &content); err != nil {
		return ""
	}
	parts := make([]string, 0, len(content.RichText))
	for _, span := range content.RichText {
		parts = append(parts, span.PlainText)
	}
	return strings.TrimSpace(strings.Join(parts, ""))
}

func (c *Client) Me(ctx context.Context) error {
	return c.call(ctx, http.MethodGet, "/users/me", nil, nil)
}

func (c *Client) call(ctx context.Context, method, path string, payload any, out any) error {
	if err := c.limiter.Wait(ctx); err != nil {
		return err
	}
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("notion %s: encode: %w", path, err)
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return fmt.Errorf("notion %s: %w", path, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Notion-Version", notionVersion)
	if payload != nil {
		req.Header.Set("content-type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("notion %s: %w", path, err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("notion %s: read body: %w", path, err)
	}
	if resp.StatusCode != http.StatusOK {
		limit := len(responseBody)
		if limit > 200 {
			limit = 200
		}
		return fmt.Errorf("notion %s: status %d: %s", path, resp.StatusCode, responseBody[:limit])
	}
	if out != nil {
		if err := json.Unmarshal(responseBody, out); err != nil {
			return fmt.Errorf("notion %s: decode: %w", path, err)
		}
	}
	return nil
}
