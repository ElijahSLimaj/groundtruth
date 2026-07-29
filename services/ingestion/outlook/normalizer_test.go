package outlook

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/attempttechnologies/company-brain/services/ingestion/connector"
)

func TestNormalizerMapsMessagesAndPreservesACL(t *testing.T) {
	t.Parallel()
	msg := message("m1", "Quote", "growth at 1499/mo")
	body, err := json.Marshal(Envelope{Message: msg})
	if err != nil {
		t.Fatal(err)
	}
	acl := connector.ACL{
		Scope:      connector.ACLScopePrincipals,
		Principals: []string{"email:ada@acme.test"},
		SourceScope: connector.SourceScope{
			Type: "outlook_thread",
			ID:   "conversation-m1",
		},
	}
	cfg := testConfig()

	ev, err := Normalizer{}.Normalize(cfg, connector.RawItem{ExternalID: "m1", Body: body}, acl)
	if err != nil {
		t.Fatal(err)
	}
	if ev.ExternalID != "m1" || ev.ThreadKey != "conversation-m1" || ev.SourceType != "outlook" {
		t.Fatalf("unexpected event %+v", ev)
	}
	if ev.AuthorRef.SourceRef != "email:sam@acme.test" {
		t.Fatalf("unexpected author %+v", ev.AuthorRef)
	}
	if !ev.OccurredAt.Equal(msg.ReceivedDateTime) {
		t.Fatalf("unexpected occurred at %v", ev.OccurredAt)
	}
	if ev.Payload.Body != "growth at 1499/mo" {
		t.Fatalf("unexpected body %q", ev.Payload.Body)
	}
	if ev.Payload.Structure["subject"] != "Quote" {
		t.Fatalf("unexpected structure %+v", ev.Payload.Structure)
	}
	if ev.ACL.Scope != connector.ACLScopePrincipals || len(ev.ACL.Principals) != 1 {
		t.Fatal("normalizer must pass the ACL through untouched")
	}
}

func TestNormalizerRejectsMissingID(t *testing.T) {
	t.Parallel()
	msg := message("", "no id", "body")
	body, err := json.Marshal(Envelope{Message: msg})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (Normalizer{}).Normalize(testConfig(), connector.RawItem{Body: body}, connector.ACL{}); err == nil {
		t.Fatal("a message without an id must never normalize")
	}
}

func TestNormalizerRejectsMissingReceivedDate(t *testing.T) {
	t.Parallel()
	msg := message("m1", "no date", "body")
	msg.ReceivedDateTime = time.Time{}
	body, err := json.Marshal(Envelope{Message: msg})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (Normalizer{}).Normalize(testConfig(), connector.RawItem{Body: body}, connector.ACL{}); err == nil {
		t.Fatal("a message without a received date must never normalize")
	}
}
