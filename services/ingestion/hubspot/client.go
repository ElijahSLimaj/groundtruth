package hubspot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/time/rate"
)

const defaultBaseURL = "https://api.hubapi.com"

var dealProperties = []string{
	"dealname",
	"amount",
	"dealstage",
	"pipeline",
	"closedate",
	"hs_lastmodifieddate",
}

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
		limiter:    rate.NewLimiter(rate.Every(time.Second/9), 9),
	}
}

type wireRecord struct {
	ID         string            `json:"id"`
	Properties map[string]string `json:"properties"`
	UpdatedAt  string            `json:"updatedAt"`
}

type wireSearch struct {
	Results []wireRecord `json:"results"`
	Paging  struct {
		Next struct {
			After string `json:"after"`
		} `json:"next"`
	} `json:"paging"`
}

func (r wireRecord) decode() (Record, error) {
	updated, err := time.Parse(time.RFC3339, r.UpdatedAt)
	if err != nil {
		return Record{}, fmt.Errorf("record %s updatedAt %q: %w", r.ID, r.UpdatedAt, err)
	}
	return Record{
		ID:         r.ID,
		Properties: r.Properties,
		UpdatedAt:  updated.UTC(),
	}, nil
}

func (c *Client) Search(ctx context.Context, gteMillis, lteMillis, after string) (Page, error) {
	filters := []map[string]any{
		{"propertyName": "hs_lastmodifieddate", "operator": "GTE", "value": gteMillis},
	}
	if lteMillis != "" {
		filters = append(filters, map[string]any{
			"propertyName": "hs_lastmodifieddate", "operator": "LTE", "value": lteMillis,
		})
	}
	body := map[string]any{
		"filterGroups": []map[string]any{{"filters": filters}},
		"sorts": []map[string]any{
			{"propertyName": "hs_lastmodifieddate", "direction": "ASCENDING"},
		},
		"properties": dealProperties,
		"limit":      100,
	}
	if after != "" {
		body["after"] = after
	}

	var out wireSearch
	if err := c.post(ctx, "/crm/v3/objects/deals/search", body, &out); err != nil {
		return Page{}, err
	}
	records := make([]Record, 0, len(out.Results))
	for _, raw := range out.Results {
		record, err := raw.decode()
		if err != nil {
			return Page{}, err
		}
		records = append(records, record)
	}
	return Page{Records: records, NextAfter: out.Paging.Next.After}, nil
}

func (c *Client) Ping(ctx context.Context) error {
	var out wireSearch
	return c.get(ctx, "/crm/v3/objects/deals?limit=1", &out)
}

func (c *Client) post(ctx context.Context, path string, body any, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("hubspot marshal: %w", err)
	}
	if err := c.limiter.Wait(ctx); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("hubspot request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, path, out)
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	if err := c.limiter.Wait(ctx); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("hubspot request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	return c.do(req, path, out)
}

func (c *Client) do(req *http.Request, path string, out any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("hubspot %s: %w", path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("hubspot %s: read body: %w", path, err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("hubspot %s: status %d: %s", path, resp.StatusCode, truncate(string(data), 200))
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("hubspot %s: decode: %w", path, err)
		}
	}
	return nil
}

func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[:limit]
}
