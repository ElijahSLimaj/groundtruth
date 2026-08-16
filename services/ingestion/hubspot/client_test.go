package hubspot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const realSearchPage1 = `{
  "total": 3,
  "results": [
    {
      "id": "512",
      "properties": {
        "amount": "1500.00",
        "closedate": "2026-09-01T00:00:00Z",
        "dealname": "Custom data integrations",
        "dealstage": "presentationscheduled",
        "hs_lastmodifieddate": "2026-08-10T14:00:00.000Z",
        "hs_object_id": "512"
      },
      "createdAt": "2026-08-01T00:00:00Z",
      "updatedAt": "2026-08-10T14:00:00Z",
      "archived": false
    }
  ],
  "paging": { "next": { "after": "10", "link": "?after=10" } }
}`

const realSearchPage2 = `{
  "total": 3,
  "results": [
    {
      "id": "513",
      "properties": {
        "amount": "9200.00",
        "dealname": "Enterprise rollout",
        "dealstage": "contractsent",
        "hs_lastmodifieddate": "2026-08-11T09:30:00.000Z"
      },
      "createdAt": "2026-08-02T00:00:00Z",
      "updatedAt": "2026-08-11T09:30:00Z",
      "archived": false
    }
  ]
}`

func testClient(baseURL string) *Client {
	c := NewClient("test-token")
	c.baseURL = baseURL
	return c
}

func TestSearchDecodesRealHubSpotPayloadAndPages(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/crm/v3/objects/deals/search" ||
			r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(string(body), `"after":"10"`) {
			_, _ = w.Write([]byte(realSearchPage2))
		} else {
			_, _ = w.Write([]byte(realSearchPage1))
		}
	}))
	defer server.Close()

	page1, err := testClient(server.URL).Search(context.Background(), "0", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page1.Records) != 1 || page1.Records[0].ID != "512" {
		t.Fatalf("unexpected page 1 %+v", page1.Records)
	}
	if page1.Records[0].Properties["amount"] != "1500.00" {
		t.Fatalf("properties not decoded: %+v", page1.Records[0].Properties)
	}
	if page1.NextAfter != "10" {
		t.Fatalf("paging cursor not read, got %q", page1.NextAfter)
	}

	page2, err := testClient(server.URL).Search(context.Background(), "0", "", "10")
	if err != nil {
		t.Fatal(err)
	}
	if page2.Records[0].ID != "513" || page2.NextAfter != "" {
		t.Fatalf("second page must end pagination, got %+v after=%q", page2.Records, page2.NextAfter)
	}
}

func TestPingHitsDealsEndpoint(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/crm/v3/objects/deals") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer server.Close()

	if err := testClient(server.URL).Ping(context.Background()); err != nil {
		t.Fatalf("ping should succeed on 200, got %v", err)
	}
}

func TestSearchSurfacesErrorStatus(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"status":"error","message":"expired"}`, http.StatusUnauthorized)
	}))
	defer server.Close()

	if _, err := testClient(server.URL).Search(context.Background(), "0", "", ""); err == nil {
		t.Fatal("a non-200 must surface as an error")
	}
}
