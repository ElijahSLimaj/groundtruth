package slack

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

const defaultBaseURL = "https://slack.com/api"

type APIError struct {
	Method string
	Code   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("slack %s: %s", e.Method, e.Code)
}

type RateLimitedError struct {
	RetryAfter time.Duration
}

func (e *RateLimitedError) Error() string {
	return fmt.Sprintf("slack rate limited, retry after %s", e.RetryAfter)
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
		limiter:    rate.NewLimiter(rate.Every(time.Second/5), 5),
	}
}

type apiResponse struct {
	OK               bool   `json:"ok"`
	Err              string `json:"error"`
	ResponseMetadata struct {
		NextCursor string `json:"next_cursor"`
	} `json:"response_metadata"`
}

func (c *Client) call(ctx context.Context, method string, params url.Values, out any) error {
	if err := c.limiter.Wait(ctx); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/"+method, strings.NewReader(params.Encode()))
	if err != nil {
		return fmt.Errorf("slack %s: %w", method, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("slack %s: %w", method, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		seconds, _ := strconv.Atoi(resp.Header.Get("Retry-After"))
		return &RateLimitedError{RetryAfter: time.Duration(seconds) * time.Second}
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack %s: unexpected status %d", method, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("slack %s: read body: %w", method, err)
	}
	var envelope apiResponse
	if err := json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("slack %s: decode: %w", method, err)
	}
	if !envelope.OK {
		return &APIError{Method: method, Code: envelope.Err}
	}
	if out != nil {
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("slack %s: decode result: %w", method, err)
		}
	}
	return nil
}

func (c *Client) AuthTest(ctx context.Context) error {
	return c.call(ctx, "auth.test", url.Values{}, nil)
}

func (c *Client) ListChannels(ctx context.Context) ([]Channel, error) {
	var channels []Channel
	cursor := ""
	for {
		params := url.Values{
			"types": {"public_channel,private_channel"},
			"limit": {"200"},
		}
		if cursor != "" {
			params.Set("cursor", cursor)
		}
		var result struct {
			apiResponse
			Channels []Channel `json:"channels"`
		}
		if err := c.call(ctx, "conversations.list", params, &result); err != nil {
			return nil, err
		}
		channels = append(channels, result.Channels...)
		cursor = result.ResponseMetadata.NextCursor
		if cursor == "" {
			return channels, nil
		}
	}
}

func (c *Client) History(ctx context.Context, channelID, oldest, latest string) ([]json.RawMessage, error) {
	var messages []json.RawMessage
	cursor := ""
	for {
		params := url.Values{
			"channel": {channelID},
			"limit":   {"200"},
		}
		if oldest != "" {
			params.Set("oldest", oldest)
		}
		if latest != "" {
			params.Set("latest", latest)
		}
		if cursor != "" {
			params.Set("cursor", cursor)
		}
		var result struct {
			apiResponse
			Messages []json.RawMessage `json:"messages"`
			HasMore  bool              `json:"has_more"`
		}
		if err := c.call(ctx, "conversations.history", params, &result); err != nil {
			return nil, err
		}
		messages = append(messages, result.Messages...)
		cursor = result.ResponseMetadata.NextCursor
		if !result.HasMore || cursor == "" {
			return messages, nil
		}
	}
}

func (c *Client) ChannelInfo(ctx context.Context, channelID string) (Channel, error) {
	var result struct {
		apiResponse
		Channel Channel `json:"channel"`
	}
	err := c.call(ctx, "conversations.info", url.Values{"channel": {channelID}}, &result)
	return result.Channel, err
}

func (c *Client) Members(ctx context.Context, channelID string) ([]string, error) {
	var members []string
	cursor := ""
	for {
		params := url.Values{
			"channel": {channelID},
			"limit":   {"200"},
		}
		if cursor != "" {
			params.Set("cursor", cursor)
		}
		var result struct {
			apiResponse
			Members []string `json:"members"`
		}
		if err := c.call(ctx, "conversations.members", params, &result); err != nil {
			return nil, err
		}
		members = append(members, result.Members...)
		cursor = result.ResponseMetadata.NextCursor
		if cursor == "" {
			return members, nil
		}
	}
}
