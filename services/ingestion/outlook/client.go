package outlook

import (
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

const defaultBaseURL = "https://graph.microsoft.com/v1.0"

const messageSelect = "id,conversationId,subject,receivedDateTime,categories,from,toRecipients,ccRecipients,body"

const textBodyPreference = `outlook.body-content-type="text"`

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

type wireRecipient struct {
	EmailAddress struct {
		Address string `json:"address"`
	} `json:"emailAddress"`
}

type wireMessage struct {
	ID               string          `json:"id"`
	ConversationID   string          `json:"conversationId"`
	Subject          string          `json:"subject"`
	ReceivedDateTime string          `json:"receivedDateTime"`
	Categories       []string        `json:"categories"`
	Removed          json.RawMessage `json:"@removed"`
	From             wireRecipient   `json:"from"`
	ToRecipients     []wireRecipient `json:"toRecipients"`
	CcRecipients     []wireRecipient `json:"ccRecipients"`
	Body             struct {
		ContentType string `json:"contentType"`
		Content     string `json:"content"`
	} `json:"body"`
}

type wireList struct {
	Value     []wireMessage `json:"value"`
	NextLink  string        `json:"@odata.nextLink"`
	DeltaLink string        `json:"@odata.deltaLink"`
}

func (w wireMessage) decode() (Message, error) {
	received, err := time.Parse(time.RFC3339, w.ReceivedDateTime)
	if err != nil {
		return Message{}, fmt.Errorf("message %s received time %q: %w", w.ID, w.ReceivedDateTime, err)
	}
	return Message{
		ID:               w.ID,
		ThreadID:         w.ConversationID,
		ReceivedDateTime: received.UTC(),
		From:             normalizeAddress(w.From.EmailAddress.Address),
		To:               recipientAddresses(w.ToRecipients),
		Cc:               recipientAddresses(w.CcRecipients),
		Subject:          w.Subject,
		Categories:       w.Categories,
		Body:             w.Body.Content,
	}, nil
}

func (c *Client) Profile(ctx context.Context) (string, error) {
	var me struct {
		Mail              string `json:"mail"`
		UserPrincipalName string `json:"userPrincipalName"`
	}
	if err := c.call(ctx, "/me", url.Values{"$select": {"mail,userPrincipalName"}}, "", &me); err != nil {
		return "", err
	}
	if me.Mail != "" {
		return me.Mail, nil
	}
	return me.UserPrincipalName, nil
}

func (c *Client) FolderDelta(ctx context.Context, folderID, deltaLink string) ([]string, string, error) {
	target := deltaLink
	if target == "" {
		target = c.folderDeltaStartURL(folderID)
	}
	var ids []string
	next, err := c.walkDelta(ctx, target, &ids)
	if err != nil {
		return nil, deltaLink, err
	}
	return ids, next, nil
}

func (c *Client) walkDelta(ctx context.Context, target string, ids *[]string) (string, error) {
	for target != "" {
		var out wireList
		if err := c.get(ctx, target, "", &out); err != nil {
			return "", err
		}
		if ids != nil {
			for _, message := range out.Value {
				if message.ID != "" && len(message.Removed) == 0 {
					*ids = append(*ids, message.ID)
				}
			}
		}
		if out.NextLink != "" {
			target = out.NextLink
			continue
		}
		return out.DeltaLink, nil
	}
	return "", fmt.Errorf("outlook delta: response carried neither next nor delta link")
}

func (c *Client) ListMessages(ctx context.Context, query string) ([]string, error) {
	params := url.Values{
		"$filter": {query},
		"$select": {"id"},
		"$top":    {"100"},
	}
	target := c.baseURL + "/me/messages?" + params.Encode()
	var ids []string
	for target != "" {
		var out wireList
		if err := c.get(ctx, target, "", &out); err != nil {
			return nil, err
		}
		for _, message := range out.Value {
			if message.ID != "" {
				ids = append(ids, message.ID)
			}
		}
		target = out.NextLink
	}
	return ids, nil
}

func (c *Client) GetMessage(ctx context.Context, id string) (Message, error) {
	var wire wireMessage
	if err := c.call(ctx, "/me/messages/"+url.PathEscape(id), url.Values{"$select": {messageSelect}}, textBodyPreference, &wire); err != nil {
		return Message{}, err
	}
	return wire.decode()
}

func (c *Client) folderDeltaStartURL(folderID string) string {
	return c.baseURL + "/me/mailFolders/" + url.PathEscape(folderID) +
		"/messages/delta?" + url.Values{"$select": {"id"}}.Encode()
}

func (c *Client) call(ctx context.Context, path string, params url.Values, prefer string, out any) error {
	target := c.baseURL + path
	if len(params) > 0 {
		target += "?" + params.Encode()
	}
	return c.get(ctx, target, prefer, out)
}

func (c *Client) get(ctx context.Context, target, prefer string, out any) error {
	if err := c.limiter.Wait(ctx); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return fmt.Errorf("outlook get %s: %w", target, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if prefer != "" {
		req.Header.Set("Prefer", prefer)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("outlook get %s: %w", target, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("outlook get %s: read body: %w", target, err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("outlook get %s: status %d: %s", target, resp.StatusCode, truncate(string(body), 200))
	}
	if out != nil {
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("outlook get %s: decode: %w", target, err)
		}
	}
	return nil
}

func recipientAddresses(recipients []wireRecipient) []string {
	if len(recipients) == 0 {
		return nil
	}
	out := make([]string, 0, len(recipients))
	for _, recipient := range recipients {
		if address := normalizeAddress(recipient.EmailAddress.Address); address != "" {
			out = append(out, address)
		}
	}
	return out
}

func normalizeAddress(address string) string {
	return strings.ToLower(strings.TrimSpace(address))
}

func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[:limit]
}
