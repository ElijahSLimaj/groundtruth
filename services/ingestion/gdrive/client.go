package gdrive

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

const defaultBaseURL = "https://www.googleapis.com/drive/v3"

const changeFileFields = "id,name,mimeType,modifiedTime,version,trashed,lastModifyingUser(emailAddress)"

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

type wireFile struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	MimeType          string `json:"mimeType"`
	ModifiedTime      string `json:"modifiedTime"`
	Version           string `json:"version"`
	Trashed           bool   `json:"trashed"`
	LastModifyingUser struct {
		EmailAddress string `json:"emailAddress"`
	} `json:"lastModifyingUser"`
}

func (w wireFile) decode() (File, error) {
	modified, err := time.Parse(time.RFC3339, w.ModifiedTime)
	if err != nil {
		return File{}, fmt.Errorf("file %s modified time %q: %w", w.ID, w.ModifiedTime, err)
	}
	version, err := strconv.ParseInt(w.Version, 10, 64)
	if err != nil {
		version = modified.UnixMilli()
	}
	return File{
		ID:           w.ID,
		Name:         w.Name,
		MimeType:     w.MimeType,
		ModifiedTime: modified,
		Version:      version,
		LastModifier: w.LastModifyingUser.EmailAddress,
		Trashed:      w.Trashed,
	}, nil
}

func (c *Client) StartPageToken(ctx context.Context) (string, error) {
	var out struct {
		StartPageToken string `json:"startPageToken"`
	}
	if err := c.call(ctx, "/changes/startPageToken", nil, &out); err != nil {
		return "", err
	}
	return out.StartPageToken, nil
}

func (c *Client) Changes(ctx context.Context, pageToken string) ([]File, string, string, error) {
	params := url.Values{
		"pageToken": {pageToken},
		"fields":    {"nextPageToken,newStartPageToken,changes(removed,file(" + changeFileFields + "))"},
	}
	var out struct {
		NextPageToken     string `json:"nextPageToken"`
		NewStartPageToken string `json:"newStartPageToken"`
		Changes           []struct {
			Removed bool     `json:"removed"`
			File    wireFile `json:"file"`
		} `json:"changes"`
	}
	if err := c.call(ctx, "/changes", params, &out); err != nil {
		return nil, "", "", err
	}
	files := make([]File, 0, len(out.Changes))
	for _, change := range out.Changes {
		if change.Removed || change.File.ID == "" {
			continue
		}
		file, err := change.File.decode()
		if err != nil {
			return nil, "", "", err
		}
		files = append(files, file)
	}
	return files, out.NextPageToken, out.NewStartPageToken, nil
}

func (c *Client) ListFiles(ctx context.Context, from, to time.Time) ([]File, error) {
	query := fmt.Sprintf(
		"modifiedTime > '%s' and modifiedTime <= '%s' and trashed = false",
		from.UTC().Format(time.RFC3339),
		to.UTC().Format(time.RFC3339),
	)
	var files []File
	pageToken := ""
	for {
		params := url.Values{
			"q":      {query},
			"fields": {"nextPageToken,files(" + changeFileFields + ")"},
		}
		if pageToken != "" {
			params.Set("pageToken", pageToken)
		}
		var out struct {
			NextPageToken string     `json:"nextPageToken"`
			Files         []wireFile `json:"files"`
		}
		if err := c.call(ctx, "/files", params, &out); err != nil {
			return nil, err
		}
		for _, wire := range out.Files {
			file, err := wire.decode()
			if err != nil {
				return nil, err
			}
			files = append(files, file)
		}
		if out.NextPageToken == "" {
			return files, nil
		}
		pageToken = out.NextPageToken
	}
}

func (c *Client) ExportText(ctx context.Context, file File) (string, error) {
	path := "/files/" + url.PathEscape(file.ID)
	params := url.Values{}
	if file.MimeType == "application/vnd.google-apps.document" {
		path += "/export"
		params.Set("mimeType", "text/plain")
	} else {
		params.Set("alt", "media")
	}
	body, err := c.raw(ctx, path, params)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (c *Client) Permissions(ctx context.Context, fileID string) ([]Permission, error) {
	params := url.Values{
		"fields": {"permissions(type,emailAddress)"},
	}
	var out struct {
		Permissions []struct {
			Type         string `json:"type"`
			EmailAddress string `json:"emailAddress"`
		} `json:"permissions"`
	}
	if err := c.call(ctx, "/files/"+url.PathEscape(fileID)+"/permissions", params, &out); err != nil {
		return nil, err
	}
	permissions := make([]Permission, 0, len(out.Permissions))
	for _, permission := range out.Permissions {
		permissions = append(permissions, Permission{
			Type:  permission.Type,
			Email: permission.EmailAddress,
		})
	}
	return permissions, nil
}

func (c *Client) About(ctx context.Context) error {
	return c.call(ctx, "/about", url.Values{"fields": {"user"}}, nil)
}

func (c *Client) call(ctx context.Context, path string, params url.Values, out any) error {
	body, err := c.raw(ctx, path, params)
	if err != nil {
		return err
	}
	if out != nil {
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("drive %s: decode: %w", path, err)
		}
	}
	return nil
}

func (c *Client) raw(ctx context.Context, path string, params url.Values) ([]byte, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	target := c.baseURL + path
	if len(params) > 0 {
		target += "?" + params.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("drive %s: %w", path, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("drive %s: %w", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("drive %s: read body: %w", path, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("drive %s: status %d: %s", path, resp.StatusCode, truncate(string(body), 200))
	}
	return body, nil
}

func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[:limit]
}
