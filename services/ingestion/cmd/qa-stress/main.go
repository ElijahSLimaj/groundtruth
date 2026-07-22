package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strconv"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/attempttechnologies/company-brain/services/ingestion/connector"
	"github.com/attempttechnologies/company-brain/services/ingestion/keys"
	"github.com/attempttechnologies/company-brain/services/ingestion/store"
)

var (
	tenantID    = uuid.MustParse("aaaaaaaa-0000-0000-0000-00000000000a")
	slackConn   = uuid.MustParse("bbbbbbbb-0000-0000-0000-000000000001")
	mailConn    = uuid.MustParse("bbbbbbbb-0000-0000-0000-000000000002")
	meetingConn = uuid.MustParse("bbbbbbbb-0000-0000-0000-000000000003")
)

type person struct {
	id    uuid.UUID
	name  string
	email string
	role  string
	slack string
}

var people = []person{
	{uuid.MustParse("cccccccc-0000-0000-0000-000000000001"), "Dana Fields", "dana@northwind.test", "admin", "U_DANA"},
	{uuid.MustParse("cccccccc-0000-0000-0000-000000000002"), "Raj Patel", "raj@northwind.test", "owner", "U_RAJ"},
	{uuid.MustParse("cccccccc-0000-0000-0000-000000000003"), "Mia Chen", "mia@northwind.test", "member", "U_MIA"},
	{uuid.MustParse("cccccccc-0000-0000-0000-000000000004"), "Tom Alvarez", "tom@northwind.test", "member", "U_TOM"},
	{uuid.MustParse("cccccccc-0000-0000-0000-000000000005"), "Sofia Rossi", "sofia@northwind.test", "member", "U_SOFIA"},
}

var slackChannels = []string{"C_SALES", "C_ENG", "C_SUPPORT", "C_GENERAL", "C_PRICING"}

var slackTemplates = []string{
	"Confirming the Growth plan is %d per month billed annually. Discounts over 15 percent need Dana to sign off.",
	"Customer %s asked about SSO. Confirmed it ships on Growth and above, not on Core.",
	"Reminder: refunds inside 30 days are automatic, after that it goes to Dana for approval.",
	"We are moving the onboarding SLA to %d business days for Growth customers.",
	"Support escalation for %s: latency spike on the ingest worker, resolved by restarting the queue drain.",
	"Decision from the pricing review: annual prepay gets 2 months free, monthly gets nothing.",
	"Heads up, the trial length is now %d days. Marketing site still says the old number.",
	"%s renewed for another year. They pushed hard on the audit export requirement.",
	"Eng note: we cap agent queries at %d per minute per tenant to protect the database.",
	"Policy: nobody ships to production on Friday after 3pm without Raj approving.",
}

var emailTemplates = []string{
	"Hi %s,\n\nFollowing up on our call. To confirm, the Growth plan is %d per month billed annually, and that includes single sign-on and all connectors.\n\nBest,\nDana",
	"Team,\n\nQ3 planning is locked. We are prioritising the Microsoft Graph connector over additional Slack features, based on %d inbound requests this quarter.\n\nRaj",
	"Hi %s,\n\nYour invoice for %d is attached. Payment terms are net 30. Late payments accrue 1.5 percent monthly.\n\nFinance",
	"All,\n\nThe security questionnaire from %s is done. We committed to dedicated tenancy for the Scale tier and a %d day breach notification window.\n\nMia",
	"Hi %s,\n\nConfirming the pilot terms: 30 days, full product, money back if the drafted canon is not useful. Cold start covers your last 90 days.\n\nDana",
}

var meetingTemplates = []string{
	"[%s] Dana: Let's lock pricing. Growth stays at %d per month.\n[%s] Raj: Agreed, but we need the discount ceiling written down.\n[%s] Dana: Fifteen percent, anything more comes to me.\n[%s] Mia: I'll update the sales deck.",
	"[%s] Raj: Standup. Ingest queue is draining at about %d events per minute.\n[%s] Tom: The embedding worker is the bottleneck, not the queue.\n[%s] Raj: Fine, we scale that first.",
	"[%s] Mia: Customer call with %s. They want audit exports before signing.\n[%s] Dana: That is Scale tier only.\n[%s] Mia: They know, they're budgeting for it.",
	"[%s] Dana: Board update. ARR is tracking to %d. Churn is under 2 percent.\n[%s] Raj: Two connectors shipped, Microsoft is next.\n[%s] Sofia: Support volume is flat despite growth.",
}

var customers = []string{"Acme Robotics", "Belmont Health", "Corvus Bank", "Delta Freight", "Evergreen Labs"}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx := context.Background()
	total := envInt("QA_EVENTS", 6000)

	adminPool, err := pgxpool.New(ctx, mustEnv("QA_DATABASE_URL"))
	if err != nil {
		return err
	}
	defer adminPool.Close()

	if err := seedCompany(ctx, adminPool); err != nil {
		return fmt.Errorf("seed company: %w", err)
	}

	workerPool, err := workerRolePool(ctx, mustEnv("QA_DATABASE_URL"))
	if err != nil {
		return err
	}
	defer workerPool.Close()

	payloads, err := buildPayloadStore(ctx, adminPool)
	if err != nil {
		return fmt.Errorf("payload store: %w", err)
	}

	rng := rand.New(rand.NewSource(42))
	events := generate(total, rng)

	fmt.Printf("generated %d events across %d days\n", len(events), 90)

	enqueueStart := time.Now()
	for _, ev := range events {
		if err := store.Enqueue(ctx, workerPool, ev); err != nil {
			return fmt.Errorf("enqueue: %w", err)
		}
	}
	enqueueDur := time.Since(enqueueStart)
	fmt.Printf("enqueued  %d events in %s (%.0f/sec)\n",
		len(events), enqueueDur.Round(time.Millisecond), float64(len(events))/enqueueDur.Seconds())

	processor := &store.Processor{Pool: workerPool, Payloads: payloads}
	var written, dupes, dead, retried int
	processStart := time.Now()
	for {
		res, err := processor.ProcessBatch(ctx, 500)
		if err != nil {
			return fmt.Errorf("process batch: %w", err)
		}
		written += res.Written
		dupes += res.Duplicates
		dead += res.DeadLettered
		retried += res.RetryScheduled
		if res.Written+res.Duplicates+res.DeadLettered+res.RetryScheduled == 0 {
			break
		}
	}
	processDur := time.Since(processStart)

	fmt.Printf("processed %d events in %s (%.0f/sec)\n",
		written+dupes+dead, processDur.Round(time.Millisecond),
		float64(written+dupes+dead)/processDur.Seconds())
	fmt.Printf("  written=%d duplicates=%d dead_lettered=%d retry_scheduled=%d\n",
		written, dupes, dead, retried)
	return nil
}

func generate(total int, rng *rand.Rand) []connector.NormalizedEvent {
	events := make([]connector.NormalizedEvent, 0, total)
	now := time.Now().UTC()
	for i := range total {
		occurred := now.Add(-time.Duration(rng.Intn(90*24)) * time.Hour)
		author := people[rng.Intn(len(people))]
		switch {
		case i%10 < 6:
			events = append(events, slackEvent(i, occurred, author, rng))
		case i%10 < 9:
			events = append(events, emailEvent(i, occurred, author, rng))
		default:
			events = append(events, meetingEvent(i, occurred, author, rng))
		}
	}
	return events
}

func slackEvent(i int, at time.Time, author person, rng *rand.Rand) connector.NormalizedEvent {
	channel := slackChannels[rng.Intn(len(slackChannels))]
	body := fmt.Sprintf(slackTemplates[rng.Intn(len(slackTemplates))],
		pick(rng, 1200, 1499, 1500, 30, 14, 5, 60))
	if rng.Intn(3) == 0 {
		body = fmt.Sprintf(slackTemplates[rng.Intn(len(slackTemplates))], customers[rng.Intn(len(customers))])
	}
	personID := author.id
	return connector.NormalizedEvent{
		TenantID:    tenantID,
		ConnectorID: slackConn,
		SourceType:  "slack",
		ExternalID:  fmt.Sprintf("slack-%d", i),
		AuthorRef:   connector.AuthorRef{PersonID: &personID, SourceRef: "slack:" + author.slack},
		ThreadKey:   fmt.Sprintf("%s:%d", channel, i/7),
		OccurredAt:  at,
		ACL: connector.ACL{
			Scope:       connector.ACLScopeTenant,
			SourceScope: connector.SourceScope{Type: "slack_channel", ID: channel, Visibility: "public"},
		},
		Payload: connector.Payload{Body: body},
	}
}

func emailEvent(i int, at time.Time, author person, rng *rand.Rand) connector.NormalizedEvent {
	body := fmt.Sprintf(emailTemplates[rng.Intn(len(emailTemplates))],
		customers[rng.Intn(len(customers))], pick(rng, 1499, 1200, 40, 30, 250000))
	personID := author.id
	return connector.NormalizedEvent{
		TenantID:    tenantID,
		ConnectorID: mailConn,
		SourceType:  "gmail",
		ExternalID:  fmt.Sprintf("mail-%d", i),
		AuthorRef:   connector.AuthorRef{PersonID: &personID, SourceRef: "gmail:" + author.email},
		ThreadKey:   fmt.Sprintf("thread-%d", i/4),
		OccurredAt:  at,
		ACL: connector.ACL{
			Scope:       connector.ACLScopePrincipals,
			Principals:  []string{author.email},
			SourceScope: connector.SourceScope{Type: "mailbox", ID: author.email},
		},
		Payload: connector.Payload{Body: body},
	}
}

func meetingEvent(i int, at time.Time, author person, rng *rand.Rand) connector.NormalizedEvent {
	stamp := at.Format("15:04")
	tpl := meetingTemplates[rng.Intn(len(meetingTemplates))]
	body := fmt.Sprintf(tpl, stamp, pick(rng, 1499, 1200, 900, 2400000), stamp, stamp, stamp)
	if len(body) > 4000 {
		body = body[:4000]
	}
	personID := author.id
	return connector.NormalizedEvent{
		TenantID:    tenantID,
		ConnectorID: meetingConn,
		SourceType:  "notion",
		ExternalID:  fmt.Sprintf("meeting-%d", i),
		AuthorRef:   connector.AuthorRef{PersonID: &personID, SourceRef: "notion:" + author.name},
		ThreadKey:   fmt.Sprintf("meeting-%d", i/3),
		OccurredAt:  at,
		ACL: connector.ACL{
			Scope:       connector.ACLScopeGroup,
			SourceScope: connector.SourceScope{Type: "notion_page", ID: "leadership", Visibility: "internal"},
		},
		Payload: connector.Payload{Body: body},
	}
}

func pick(rng *rand.Rand, options ...int) int {
	return options[rng.Intn(len(options))]
}

func seedCompany(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, `
		insert into tenants (id, name, tier) values ($1, 'Northwind Robotics', 'growth')
		on conflict (id) do nothing`, tenantID); err != nil {
		return err
	}
	for _, p := range people {
		if _, err := pool.Exec(ctx, `
			insert into people (id, tenant_id, email, display_name, role)
			values ($1, $2, $3, $4, $5) on conflict (id) do nothing`,
			p.id, tenantID, p.email, p.name, p.role); err != nil {
			return err
		}
	}
	conns := []struct {
		id  uuid.UUID
		typ string
	}{{slackConn, "slack"}, {mailConn, "gmail"}, {meetingConn, "notion"}}
	for _, c := range conns {
		if _, err := pool.Exec(ctx, `
			insert into connectors (id, tenant_id, source_type, status, config)
			values ($1, $2, $3, 'active', '{}'::jsonb) on conflict (id) do nothing`,
			c.id, tenantID, c.typ); err != nil {
			return err
		}
	}
	return nil
}

func workerRolePool(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, "set role "+pgx.Identifier{"brain_worker"}.Sanitize())
		return err
	}
	return pgxpool.NewWithConfig(ctx, cfg)
}

func buildPayloadStore(ctx context.Context, pool *pgxpool.Pool) (store.PayloadStore, error) {
	wrapper, err := keys.NewAESWrapper(mustEnv("QA_MASTER_KEY"))
	if err != nil {
		return nil, err
	}
	endpoint := mustEnv("QA_S3_ENDPOINT")
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(os.Getenv("QA_S3_REGION")))
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = &endpoint
		o.UsePathStyle = true
	})
	return &store.EncryptedPayloadStore{
		Blobs: &store.S3BlobStore{Client: client, Bucket: mustEnv("QA_S3_BUCKET")},
		Keys:  &keys.Service{Pool: pool, Wrapper: wrapper},
	}, nil
}

func mustEnv(key string) string {
	value := os.Getenv(key)
	if value == "" {
		log.Fatalf("%s is required", key)
	}
	return value
}

func envInt(key string, fallback int) int {
	if raw := os.Getenv(key); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			return n
		}
	}
	return fallback
}
