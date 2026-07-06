package slack

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/attempttechnologies/company-brain/services/ingestion/connector"
)

type histCall struct {
	channel string
	oldest  string
	latest  string
}

type fakeAPI struct {
	channels    []Channel
	history     map[string][]json.RawMessage
	histErr     map[string]error
	members     map[string][]string
	membersErr  error
	authErr     error
	infoCalls   int
	memberCalls int
	histCalls   []histCall
}

func (f *fakeAPI) AuthTest(context.Context) error {
	return f.authErr
}

func (f *fakeAPI) ListChannels(context.Context) ([]Channel, error) {
	return f.channels, nil
}

func (f *fakeAPI) History(_ context.Context, channelID, oldest, latest string) ([]json.RawMessage, error) {
	f.histCalls = append(f.histCalls, histCall{channel: channelID, oldest: oldest, latest: latest})
	if err := f.histErr[channelID]; err != nil {
		return nil, err
	}
	return f.history[channelID], nil
}

func (f *fakeAPI) ChannelInfo(_ context.Context, channelID string) (Channel, error) {
	f.infoCalls++
	for _, ch := range f.channels {
		if ch.ID == channelID {
			return ch, nil
		}
	}
	return Channel{}, errors.New("channel_not_found")
}

func (f *fakeAPI) Members(_ context.Context, channelID string) ([]string, error) {
	f.memberCalls++
	if f.membersErr != nil {
		return nil, f.membersErr
	}
	return f.members[channelID], nil
}

type collectSink struct {
	items  []connector.RawItem
	failOn string
}

func (s *collectSink) Emit(_ context.Context, item connector.RawItem) error {
	if s.failOn != "" && item.ExternalID == s.failOn {
		return errors.New("sink rejected item")
	}
	s.items = append(s.items, item)
	return nil
}

func msg(ts, user, text string) json.RawMessage {
	encoded, _ := json.Marshal(map[string]string{"ts": ts, "user": user, "text": text})
	return encoded
}

func testConfig() connector.Config {
	return connector.Config{
		ConnectorID: uuid.New(),
		TenantID:    uuid.New(),
		SourceType:  SourceType,
		Settings:    map[string]any{"token": "xoxb-test"},
	}
}

func newTestConnector(fake *fakeAPI) *Connector {
	return New(func(string) API { return fake })
}

func externalIDs(items []connector.RawItem) []string {
	ids := make([]string, len(items))
	for i, item := range items {
		ids[i] = item.ExternalID
	}
	return ids
}

func TestPollEmitsOldestFirstAndTracksPerChannelCursor(t *testing.T) {
	t.Parallel()
	fake := &fakeAPI{
		channels: []Channel{{ID: "C2", Name: "random"}, {ID: "C1", Name: "general"}},
		history: map[string][]json.RawMessage{
			"C1": {msg("300.000000", "U1", "newest"), msg("100.000000", "U1", "oldest")},
			"C2": {msg("200.000000", "U2", "only")},
		},
	}
	c := newTestConnector(fake)
	sink := &collectSink{}

	next, err := c.Poll(context.Background(), testConfig(), "", sink)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"C1:100.000000", "C1:300.000000", "C2:200.000000"}
	got := externalIDs(sink.items)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("expected %v, got %v", want, got)
	}

	decoded, err := decodeCursor(next)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Channels["C1"] != "300.000000" || decoded.Channels["C2"] != "200.000000" {
		t.Fatalf("unexpected cursor %+v", decoded)
	}
}

func TestPollResumesFromPersistedCursor(t *testing.T) {
	t.Parallel()
	fake := &fakeAPI{
		channels: []Channel{{ID: "C1"}},
		history:  map[string][]json.RawMessage{"C1": {}},
	}
	c := newTestConnector(fake)

	cursor, err := encodeCursor(pollCursor{Channels: map[string]string{"C1": "300.000000"}})
	if err != nil {
		t.Fatal(err)
	}
	next, err := c.Poll(context.Background(), testConfig(), cursor, &collectSink{})
	if err != nil {
		t.Fatal(err)
	}

	if len(fake.histCalls) != 1 || fake.histCalls[0].oldest != "300.000000" {
		t.Fatalf("expected history from cursor ts, got %+v", fake.histCalls)
	}
	decoded, _ := decodeCursor(next)
	if decoded.Channels["C1"] != "300.000000" {
		t.Fatalf("empty page must keep the cursor, got %+v", decoded)
	}
}

func TestPollExcludesDMsGroupDMsAndArchivedChannels(t *testing.T) {
	t.Parallel()
	fake := &fakeAPI{
		channels: []Channel{
			{ID: "C1", Name: "general"},
			{ID: "D1", IsIM: true},
			{ID: "G1", IsMpIM: true},
			{ID: "C9", IsArchived: true},
		},
		history: map[string][]json.RawMessage{
			"C1": {msg("100.000000", "U1", "ok")},
			"D1": {msg("101.000000", "U1", "dm, never ingested")},
			"G1": {msg("102.000000", "U1", "group dm, never ingested")},
			"C9": {msg("103.000000", "U1", "archived")},
		},
	}
	c := newTestConnector(fake)
	sink := &collectSink{}

	if _, err := c.Poll(context.Background(), testConfig(), "", sink); err != nil {
		t.Fatal(err)
	}
	if len(sink.items) != 1 || sink.items[0].ExternalID != "C1:100.000000" {
		t.Fatalf("expected only the public channel message, got %v", externalIDs(sink.items))
	}
	if len(fake.histCalls) != 1 || fake.histCalls[0].channel != "C1" {
		t.Fatalf("excluded channels must not be fetched at all, got %+v", fake.histCalls)
	}
}

func TestPollFailureReturnsOriginalCursor(t *testing.T) {
	t.Parallel()
	original, err := encodeCursor(pollCursor{Channels: map[string]string{"C1": "100.000000"}})
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		fake *fakeAPI
		sink *collectSink
	}{
		{
			name: "history_error",
			fake: &fakeAPI{
				channels: []Channel{{ID: "C1"}, {ID: "C2"}},
				history:  map[string][]json.RawMessage{"C1": {msg("200.000000", "U1", "fine")}},
				histErr:  map[string]error{"C2": errors.New("slack unavailable")},
			},
			sink: &collectSink{},
		},
		{
			name: "sink_error",
			fake: &fakeAPI{
				channels: []Channel{{ID: "C1"}, {ID: "C2"}},
				history: map[string][]json.RawMessage{
					"C1": {msg("200.000000", "U1", "fine")},
					"C2": {msg("300.000000", "U1", "rejected downstream")},
				},
			},
			sink: &collectSink{failOn: "C2:300.000000"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := newTestConnector(tc.fake)
			next, err := c.Poll(context.Background(), testConfig(), original, tc.sink)
			if err == nil {
				t.Fatal("expected the poll to fail")
			}
			if next != original {
				t.Fatalf("a failed poll must return the original cursor, got %q", next)
			}
		})
	}
}

func envelopeItem(t *testing.T, channelID string) connector.RawItem {
	t.Helper()
	body, err := json.Marshal(Envelope{
		Channel: EnvelopeChannel{ID: channelID},
		Message: msg("100.000000", "U1", "hello"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return connector.RawItem{ExternalID: channelID + ":100.000000", Body: body}
}

func TestResolveACLPublicChannelIsTenantScoped(t *testing.T) {
	t.Parallel()
	fake := &fakeAPI{channels: []Channel{{ID: "C1", Name: "general"}}}
	c := newTestConnector(fake)

	acl, err := c.ResolveACL(context.Background(), testConfig(), envelopeItem(t, "C1"))
	if err != nil {
		t.Fatal(err)
	}
	if acl.Scope != connector.ACLScopeTenant {
		t.Fatalf("expected tenant scope, got %q", acl.Scope)
	}
	if acl.SourceScope.Type != "slack_channel" || acl.SourceScope.ID != "C1" || acl.SourceScope.Visibility != "public" {
		t.Fatalf("unexpected source scope %+v", acl.SourceScope)
	}
	if fake.memberCalls != 0 {
		t.Fatal("public channels must not trigger membership lookups")
	}
}

func TestResolveACLPrivateChannelCapturesMembershipAndNeverWidens(t *testing.T) {
	t.Parallel()
	fake := &fakeAPI{
		channels: []Channel{{ID: "P1", Name: "founders", IsPrivate: true}},
		members:  map[string][]string{"P1": {"U2", "U1"}},
	}
	c := newTestConnector(fake)

	acl, err := c.ResolveACL(context.Background(), testConfig(), envelopeItem(t, "P1"))
	if err != nil {
		t.Fatal(err)
	}
	if acl.Scope != connector.ACLScopePrincipals {
		t.Fatalf("private channel ACL widened to %q", acl.Scope)
	}
	if len(acl.Principals) != 2 || acl.Principals[0] != "slack:U1" || acl.Principals[1] != "slack:U2" {
		t.Fatalf("unexpected principals %v", acl.Principals)
	}
	if acl.SourceScope.Visibility != "private" {
		t.Fatalf("unexpected source scope %+v", acl.SourceScope)
	}
}

func TestResolveACLDirectMessagesAreRejected(t *testing.T) {
	t.Parallel()
	fake := &fakeAPI{channels: []Channel{{ID: "D1", IsIM: true, IsPrivate: true}}}
	c := newTestConnector(fake)

	_, err := c.ResolveACL(context.Background(), testConfig(), envelopeItem(t, "D1"))
	if err == nil || !strings.Contains(err.Error(), "direct message") {
		t.Fatalf("expected dm rejection, got %v", err)
	}
}

func TestResolveACLMembershipFailureIsStuckNotDefaulted(t *testing.T) {
	t.Parallel()
	fake := &fakeAPI{
		channels:   []Channel{{ID: "P1", IsPrivate: true}},
		membersErr: errors.New("slack unavailable"),
	}
	c := newTestConnector(fake)

	_, err := c.ResolveACL(context.Background(), testConfig(), envelopeItem(t, "P1"))
	if err == nil {
		t.Fatal("a private channel without resolvable members must never produce an ACL")
	}
}

func TestResolveACLPrivateChannelWithNoMembersIsStuck(t *testing.T) {
	t.Parallel()
	fake := &fakeAPI{
		channels: []Channel{{ID: "P1", IsPrivate: true}},
		members:  map[string][]string{"P1": {}},
	}
	c := newTestConnector(fake)

	_, err := c.ResolveACL(context.Background(), testConfig(), envelopeItem(t, "P1"))
	if err == nil {
		t.Fatal("an empty membership list must never default to visible")
	}
}

func TestResolveACLCachesChannelLookups(t *testing.T) {
	t.Parallel()
	fake := &fakeAPI{
		channels: []Channel{{ID: "P1", IsPrivate: true}},
		members:  map[string][]string{"P1": {"U1"}},
	}
	c := newTestConnector(fake)
	cfg := testConfig()

	for range 3 {
		if _, err := c.ResolveACL(context.Background(), cfg, envelopeItem(t, "P1")); err != nil {
			t.Fatal(err)
		}
	}
	if fake.infoCalls != 1 || fake.memberCalls != 1 {
		t.Fatalf("expected one lookup within the cache ttl, got info=%d members=%d", fake.infoCalls, fake.memberCalls)
	}
}

func TestBackfillPassesWindowBounds(t *testing.T) {
	t.Parallel()
	fake := &fakeAPI{
		channels: []Channel{{ID: "C1"}},
		history:  map[string][]json.RawMessage{"C1": {msg("150.000000", "U1", "old")}},
	}
	c := newTestConnector(fake)
	sink := &collectSink{}
	window := connector.BackfillWindow{
		From: time.Unix(100, 0).UTC(),
		To:   time.Unix(200, 0).UTC(),
	}

	if err := c.Backfill(context.Background(), testConfig(), window, sink); err != nil {
		t.Fatal(err)
	}
	if len(fake.histCalls) != 1 {
		t.Fatalf("expected one history call, got %+v", fake.histCalls)
	}
	call := fake.histCalls[0]
	if call.oldest != "100.000000" || call.latest != "200.000000" {
		t.Fatalf("unexpected window bounds %+v", call)
	}
	if len(sink.items) != 1 {
		t.Fatalf("expected 1 backfilled item, got %d", len(sink.items))
	}
}

func TestHealthCheckReflectsAuthState(t *testing.T) {
	t.Parallel()
	healthy := newTestConnector(&fakeAPI{})
	if h := healthy.HealthCheck(context.Background(), testConfig()); h.State != connector.HealthLive {
		t.Fatalf("expected live, got %+v", h)
	}

	broken := newTestConnector(&fakeAPI{authErr: &APIError{Method: "auth.test", Code: "token_revoked"}})
	h := broken.HealthCheck(context.Background(), testConfig())
	if h.State != connector.HealthDegraded || !strings.Contains(h.Message, "token_revoked") {
		t.Fatalf("expected degraded with reason, got %+v", h)
	}
}

func TestMissingTokenFailsEverything(t *testing.T) {
	t.Parallel()
	c := newTestConnector(&fakeAPI{})
	cfg := testConfig()
	cfg.Settings = map[string]any{}

	if _, err := c.Poll(context.Background(), cfg, "", &collectSink{}); err == nil {
		t.Fatal("expected poll to fail without a token")
	}
	if h := c.HealthCheck(context.Background(), cfg); h.State != connector.HealthDegraded {
		t.Fatalf("expected degraded without a token, got %+v", h)
	}
}
