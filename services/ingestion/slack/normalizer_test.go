package slack

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/attempttechnologies/company-brain/services/ingestion/connector"
)

type goldenInput struct {
	ExternalID string        `json:"external_id"`
	ACL        connector.ACL `json:"acl"`
	Envelope   Envelope      `json:"envelope"`
}

func goldenConfig() connector.Config {
	return connector.Config{
		ConnectorID: uuid.MustParse("33333333-0000-0000-0000-000000000001"),
		TenantID:    uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		SourceType:  SourceType,
		Settings:    map[string]any{"token": "xoxb-test"},
	}
}

func TestNormalizeGoldenFiles(t *testing.T) {
	t.Parallel()
	inputs, err := filepath.Glob(filepath.Join("testdata", "*.input.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) == 0 {
		t.Fatal("no golden inputs found in testdata")
	}

	for _, inputPath := range inputs {
		name := strings.TrimSuffix(filepath.Base(inputPath), ".input.json")
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			raw, err := os.ReadFile(inputPath)
			if err != nil {
				t.Fatal(err)
			}
			var input goldenInput
			if err := json.Unmarshal(raw, &input); err != nil {
				t.Fatal(err)
			}
			body, err := json.Marshal(input.Envelope)
			if err != nil {
				t.Fatal(err)
			}
			item := connector.RawItem{ExternalID: input.ExternalID, Body: body}

			got, err := Normalizer{}.Normalize(goldenConfig(), item, input.ACL)
			if err != nil {
				t.Fatal(err)
			}
			gotJSON, err := json.MarshalIndent(got, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			gotJSON = append(gotJSON, '\n')

			goldenPath := filepath.Join("testdata", name+".golden.json")
			if os.Getenv("UPDATE_GOLDEN") == "1" {
				if err := os.WriteFile(goldenPath, gotJSON, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatal(err)
			}
			if string(gotJSON) != string(want) {
				t.Fatalf("golden mismatch for %s\ngot:\n%s\nwant:\n%s", name, gotJSON, want)
			}
		})
	}
}

func TestNormalizePreservesACLExactly(t *testing.T) {
	t.Parallel()
	acl := connector.ACL{
		Scope:       connector.ACLScopePrincipals,
		Principals:  []string{"slack:U1", "slack:U2"},
		SourceScope: connector.SourceScope{Type: "slack_channel", ID: "P1", Visibility: "private"},
	}
	body, err := json.Marshal(Envelope{
		Channel: EnvelopeChannel{ID: "P1", Name: "founders"},
		Message: msg("100.000000", "U1", "private strategy note"),
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := Normalizer{}.Normalize(goldenConfig(),
		connector.RawItem{ExternalID: "P1:100.000000", Body: body}, acl)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.ACL, acl) {
		t.Fatalf("normalizer changed the ACL: %+v", got.ACL)
	}
}

func TestNormalizeRejectsMessagesWithoutTS(t *testing.T) {
	t.Parallel()
	body, err := json.Marshal(Envelope{
		Channel: EnvelopeChannel{ID: "C1"},
		Message: json.RawMessage(`{"user": "U1", "text": "no ts"}`),
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = Normalizer{}.Normalize(goldenConfig(),
		connector.RawItem{ExternalID: "C1:?", Body: body}, connector.ACL{Scope: connector.ACLScopeTenant})
	if err == nil {
		t.Fatal("expected an error for a message without ts")
	}
}
