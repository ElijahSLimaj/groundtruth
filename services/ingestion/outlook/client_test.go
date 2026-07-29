package outlook

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const realMessagePayload = `{
  "@odata.context":"https://graph.microsoft.com/v1.0/$metadata#users('7f180cbb')/messages/$entity",
  "id":"AAMkADhMGAAA=",
  "createdDateTime":"2018-09-09T03:15:05Z",
  "receivedDateTime":"2018-09-09T03:15:08Z",
  "sentDateTime":"2018-09-09T03:15:06Z",
  "hasAttachments":false,
  "subject":"9/9/2018: concert",
  "bodyPreview":"The group represents Nevada.",
  "conversationId":"AAQkADOUpag6yWs=",
  "isRead":true,
  "categories":[],
  "body":{
    "contentType":"text",
    "content":"The group represents Nevada."
  },
  "sender":{"emailAddress":{"name":"Adele Vance","address":"adelev@contoso.com"}},
  "from":{"emailAddress":{"name":"Adele Vance","address":"adelev@contoso.com"}},
  "toRecipients":[{"emailAddress":{"name":"Alex Wilber","address":"AlexW@contoso.com"}}],
  "ccRecipients":[{"emailAddress":{"name":"Lee Gu","address":"LeeG@contoso.com"}}],
  "bccRecipients":[],
  "replyTo":[]
}`

func testClient(baseURL string) *Client {
	client := NewClient("test-token")
	client.baseURL = baseURL
	return client
}

func TestGetMessageDecodesRealGraphPayload(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/me/messages/") {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(realMessagePayload))
	}))
	defer server.Close()

	message, err := testClient(server.URL).GetMessage(context.Background(), "AAMkADhMGAAA=")
	if err != nil {
		t.Fatal(err)
	}
	if message.ID != "AAMkADhMGAAA=" {
		t.Fatalf("id: got %q", message.ID)
	}
	if message.ThreadID != "AAQkADOUpag6yWs=" {
		t.Fatalf("conversationId must map to ThreadID, got %q", message.ThreadID)
	}
	if message.From != "adelev@contoso.com" {
		t.Fatalf("from.emailAddress.address must flatten to From, got %q", message.From)
	}
	if len(message.To) != 1 || message.To[0] != "alexw@contoso.com" {
		t.Fatalf("toRecipients must flatten and lowercase, got %v", message.To)
	}
	if len(message.Cc) != 1 || message.Cc[0] != "leeg@contoso.com" {
		t.Fatalf("ccRecipients must flatten and lowercase, got %v", message.Cc)
	}
	if !message.ReceivedDateTime.Equal(mustParse(t, "2018-09-09T03:15:08Z")) {
		t.Fatalf("receivedDateTime not parsed, got %v", message.ReceivedDateTime)
	}
	if message.Subject != "9/9/2018: concert" || message.Body != "The group represents Nevada." {
		t.Fatalf("subject/body mismatch: %q / %q", message.Subject, message.Body)
	}
}

func TestFolderDeltaPagesRealGraphPayloadAndSkipsRemoved(t *testing.T) {
	t.Parallel()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/me/mailFolders/inbox/messages/delta"):
			_, _ = w.Write([]byte(`{
              "@odata.context":"https://graph.microsoft.com/v1.0/$metadata#Collection(message)",
              "@odata.nextLink":"` + server.URL + `/page2?$skiptoken=abc",
              "value":[
                {"@odata.type":"#microsoft.graph.message","id":"A"},
                {"@odata.type":"#microsoft.graph.message","id":"B"}
              ]
            }`))
		case strings.HasPrefix(r.URL.Path, "/page2"):
			_, _ = w.Write([]byte(`{
              "@odata.context":"https://graph.microsoft.com/v1.0/$metadata#Collection(message)",
              "@odata.deltaLink":"` + server.URL + `/delta?$deltatoken=xyz",
              "value":[
                {"@odata.type":"#microsoft.graph.message","id":"C","@removed":{"reason":"deleted"}},
                {"@odata.type":"#microsoft.graph.message","id":"D"}
              ]
            }`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ids, deltaLink, err := testClient(server.URL).FolderDelta(context.Background(), "inbox", "")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"A", "B", "D"}
	if len(ids) != len(want) {
		t.Fatalf("removed message must be skipped, got %v", ids)
	}
	for i, id := range want {
		if ids[i] != id {
			t.Fatalf("expected %v, got %v", want, ids)
		}
	}
	if deltaLink != server.URL+"/delta?$deltatoken=xyz" {
		t.Fatalf("must return the final deltaLink, got %q", deltaLink)
	}
}

func TestFolderDeltaFollowsStoredDeltaLink(t *testing.T) {
	t.Parallel()
	var hit string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = r.URL.Path + "?" + r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"@odata.deltaLink":"next","value":[]}`))
	}))
	defer server.Close()

	if _, _, err := testClient(server.URL).FolderDelta(context.Background(), "inbox", server.URL+"/delta?$deltatoken=stored"); err != nil {
		t.Fatal(err)
	}
	if hit != "/delta?$deltatoken=stored" {
		t.Fatalf("a stored delta link must be replayed verbatim, hit %q", hit)
	}
}

func TestGetMessageSurfacesGraphErrorStatus(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":{"code":"ErrorItemNotFound"}}`, http.StatusNotFound)
	}))
	defer server.Close()

	if _, err := testClient(server.URL).GetMessage(context.Background(), "missing"); err == nil {
		t.Fatal("a non-200 Graph response must surface as an error, not a zero message")
	}
}

func mustParse(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
