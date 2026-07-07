package gmail

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

const defaultBaseURL = "https://gmail.googleapis.com/gmail/v1/users/me"

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
		limiter:    rate.NewLimiter(rate.Every(time.Second/10), 10),
	}
}

func (c *Client) call(ctx context.Context, path string, params url.Values, out any) error {
	if err := c.limiter.Wait(ctx); err != nil {
		return err
	}
	target := c.baseURL + path
	if len(params) > 0 {
		target += "?" + params.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return fmt.Errorf("gmail %s: %w", path, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("gmail %s: %w", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("gmail %s: read body: %w", path, err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gmail %s: status %d: %s", path, resp.StatusCode, truncate(string(body), 200))
	}
	if out != nil {
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("gmail %s: decode: %w", path, err)
		}
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func (c *Client) Profile(ctx context.Context) (string, uint64, error) {
	var result struct {
		EmailAddress string `json:"emailAddress"`
		HistoryID    string `json:"historyId"`
	}
	if err := c.call(ctx, "/profile", nil, &result); err != nil {
		return "", 0, err
	}
	historyID, err := strconv.ParseUint(result.HistoryID, 10, 64)
	if err != nil {
		return "", 0, fmt.Errorf("gmail profile: parse historyId %q: %w", result.HistoryID, err)
	}
	return result.EmailAddress, historyID, nil
}

func (c *Client) ListHistory(ctx context.Context, startHistoryID uint64) ([]string, uint64, error) {
	var ids []string
	newest := startHistoryID
	pageToken := ""
	for {
		params := url.Values{
			"startHistoryId": {strconv.FormatUint(startHistoryID, 10)},
			"historyTypes":   {"messageAdded"},
			"maxResults":     {"100"},
		}
		if pageToken != "" {
			params.Set("pageToken", pageToken)
		}
		var result struct {
			History []struct {
				MessagesAdded []struct {
					Message struct {
						ID string `json:"id"`
					} `json:"message"`
				} `json:"messagesAdded"`
			} `json:"history"`
			HistoryID     string `json:"historyId"`
			NextPageToken string `json:"nextPageToken"`
		}
		if err := c.call(ctx, "/history", params, &result); err != nil {
			return nil, startHistoryID, err
		}
		for _, h := range result.History {
			for _, added := range h.MessagesAdded {
				ids = append(ids, added.Message.ID)
			}
		}
		if result.HistoryID != "" {
			if parsed, err := strconv.ParseUint(result.HistoryID, 10, 64); err == nil && parsed > newest {
				newest = parsed
			}
		}
		pageToken = result.NextPageToken
		if pageToken == "" {
			return ids, newest, nil
		}
	}
}

func (c *Client) ListMessages(ctx context.Context, query string) ([]string, error) {
	var ids []string
	pageToken := ""
	for {
		params := url.Values{"q": {query}, "maxResults": {"100"}}
		if pageToken != "" {
			params.Set("pageToken", pageToken)
		}
		var result struct {
			Messages []struct {
				ID string `json:"id"`
			} `json:"messages"`
			NextPageToken string `json:"nextPageToken"`
		}
		if err := c.call(ctx, "/messages", params, &result); err != nil {
			return nil, err
		}
		for _, m := range result.Messages {
			ids = append(ids, m.ID)
		}
		pageToken = result.NextPageToken
		if pageToken == "" {
			return ids, nil
		}
	}
}

type payloadPart struct {
	MimeType string `json:"mimeType"`
	Body     struct {
		Data string `json:"data"`
	} `json:"body"`
	Parts []payloadPart `json:"parts"`
}

func (c *Client) GetMessage(ctx context.Context, id string) (Message, error) {
	var result struct {
		ID           string   `json:"id"`
		ThreadID     string   `json:"threadId"`
		LabelIDs     []string `json:"labelIds"`
		InternalDate string   `json:"internalDate"`
		Payload      struct {
			Headers []struct {
				Name  string `json:"name"`
				Value string `json:"value"`
			} `json:"headers"`
			payloadPart
		} `json:"payload"`
	}
	if err := c.call(ctx, "/messages/"+id, url.Values{"format": {"full"}}, &result); err != nil {
		return Message{}, err
	}

	msg := Message{ID: result.ID, ThreadID: result.ThreadID, LabelIDs: result.LabelIDs}
	if millis, err := strconv.ParseInt(result.InternalDate, 10, 64); err == nil {
		msg.InternalDate = time.UnixMilli(millis).UTC()
	}
	for _, header := range result.Payload.Headers {
		switch strings.ToLower(header.Name) {
		case "from":
			msg.From = firstAddress(header.Value)
		case "to":
			msg.To = addressList(header.Value)
		case "cc":
			msg.Cc = addressList(header.Value)
		case "subject":
			msg.Subject = header.Value
		}
	}
	msg.Body = extractText(result.Payload.payloadPart)
	return msg, nil
}

func extractText(part payloadPart) string {
	if part.MimeType == "text/plain" && part.Body.Data != "" {
		decoded, err := base64.URLEncoding.WithPadding(base64.NoPadding).DecodeString(part.Body.Data)
		if err == nil {
			return string(decoded)
		}
	}
	for _, child := range part.Parts {
		if text := extractText(child); text != "" {
			return text
		}
	}
	return ""
}

func firstAddress(value string) string {
	addresses := addressList(value)
	if len(addresses) == 0 {
		return ""
	}
	return addresses[0]
}

func addressList(value string) []string {
	parsed, err := mail.ParseAddressList(value)
	if err != nil {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return nil
		}
		return []string{strings.ToLower(trimmed)}
	}
	out := make([]string, 0, len(parsed))
	for _, address := range parsed {
		out = append(out, strings.ToLower(address.Address))
	}
	return out
}
