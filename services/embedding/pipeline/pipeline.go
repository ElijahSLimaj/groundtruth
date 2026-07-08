package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/attempttechnologies/company-brain/services/embedding/chunker"
	"github.com/attempttechnologies/company-brain/services/embedding/payload"
	"github.com/attempttechnologies/company-brain/services/embedding/provider"
)

type Pipeline struct {
	Pool      *pgxpool.Pool
	Provider  provider.Provider
	Payloads  payload.Reader
	BatchSize int
	ScanLimit int
}

type Result struct {
	EventsScanned      int
	ChunksWritten      int
	DeadLettered       int
	StatementsEmbedded int
}

type scannedEvent struct {
	ID              uuid.UUID
	TenantID        uuid.UUID
	SourceType      string
	ThreadKey       string
	OccurredAt      time.Time
	IngestedAt      time.Time
	ACL             []byte
	PayloadRef      string
	AuthorSourceRef string
}

type payloadBody struct {
	Body string `json:"body"`
}

func (p *Pipeline) batchSize() int {
	if p.BatchSize > 0 {
		return p.BatchSize
	}
	return 64
}

func (p *Pipeline) scanLimit() int {
	if p.ScanLimit > 0 {
		return p.ScanLimit
	}
	return 500
}

func (p *Pipeline) Run(ctx context.Context) (Result, error) {
	var result Result
	model := p.Provider.ModelID()

	embedded, err := p.embedStatements(ctx, model)
	if err != nil {
		return result, err
	}
	result.StatementsEmbedded = embedded

	wmTime, wmID, err := p.loadWatermark(ctx, model)
	if err != nil {
		return result, err
	}

	scanned, err := p.scanEvents(ctx, wmTime, wmID)
	if err != nil {
		return result, err
	}
	result.EventsScanned = len(scanned)
	if len(scanned) == 0 {
		return result, nil
	}

	byTenant := map[uuid.UUID][]scannedEvent{}
	for _, ev := range scanned {
		byTenant[ev.TenantID] = append(byTenant[ev.TenantID], ev)
	}

	for tenantID, tenantEvents := range byTenant {
		written, deadLettered, err := p.processTenant(ctx, model, tenantID, tenantEvents)
		if err != nil {
			return result, fmt.Errorf("tenant %s: %w", tenantID, err)
		}
		result.ChunksWritten += written
		result.DeadLettered += deadLettered
	}

	last := scanned[len(scanned)-1]
	if err := p.saveWatermark(ctx, model, last.IngestedAt, last.ID); err != nil {
		return result, err
	}
	return result, nil
}

func (p *Pipeline) embedStatements(ctx context.Context, model string) (int, error) {
	rows, err := p.Pool.Query(ctx, `
		select cv.id, cv.tenant_id, cv.statement
		from canon_versions cv
		join canon_entries ce on ce.current_version_id = cv.id
		where ce.status in ('active', 'decayed')
		  and not exists (
		    select 1 from canon_statement_embeddings cse
		    where cse.version_id = cv.id and cse.embedding_model = $1
		  )
		limit 200
	`, model)
	if err != nil {
		return 0, fmt.Errorf("scan statements: %w", err)
	}
	defer rows.Close()

	type statement struct {
		versionID uuid.UUID
		tenantID  uuid.UUID
		text      string
	}
	var statements []statement
	for rows.Next() {
		var s statement
		if err := rows.Scan(&s.versionID, &s.tenantID, &s.text); err != nil {
			return 0, fmt.Errorf("scan statement row: %w", err)
		}
		statements = append(statements, s)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("scan statements: %w", err)
	}
	if len(statements) == 0 {
		return 0, nil
	}

	texts := make([]string, len(statements))
	for i, s := range statements {
		texts[i] = s.text
	}
	vectors, err := p.Provider.Embed(ctx, texts)
	if err != nil {
		return 0, fmt.Errorf("embed statements: %w", err)
	}

	for i, s := range statements {
		if _, err := p.Pool.Exec(ctx, `
			insert into canon_statement_embeddings (version_id, embedding_model, tenant_id, embedding)
			values ($1, $2, $3, $4::extensions.vector)
			on conflict (version_id, embedding_model) do nothing
		`, s.versionID, model, s.tenantID, vectorLiteral(vectors[i])); err != nil {
			return 0, fmt.Errorf("insert statement embedding: %w", err)
		}
	}
	return len(statements), nil
}

func (p *Pipeline) loadWatermark(ctx context.Context, model string) (time.Time, uuid.UUID, error) {
	var t time.Time
	var id uuid.UUID
	err := p.Pool.QueryRow(ctx,
		`select last_ingested_at, last_event_id from embedding_watermark where embedding_model = $1`,
		model,
	).Scan(&t, &id)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, uuid.Nil, nil
	}
	if err != nil {
		return t, id, fmt.Errorf("load watermark: %w", err)
	}
	return t, id, nil
}

func (p *Pipeline) saveWatermark(ctx context.Context, model string, t time.Time, id uuid.UUID) error {
	_, err := p.Pool.Exec(ctx, `
		insert into embedding_watermark (embedding_model, last_ingested_at, last_event_id)
		values ($1, $2, $3)
		on conflict (embedding_model) do update
		set last_ingested_at = excluded.last_ingested_at,
		    last_event_id = excluded.last_event_id,
		    updated_at = now()
	`, model, t, id)
	if err != nil {
		return fmt.Errorf("save watermark: %w", err)
	}
	return nil
}

const eventColumns = `id, tenant_id, source_type, thread_key, occurred_at, ingested_at, acl::text, payload_ref, coalesce(author_source_ref, '')`

func (p *Pipeline) scanEvents(ctx context.Context, wmTime time.Time, wmID uuid.UUID) ([]scannedEvent, error) {
	rows, err := p.Pool.Query(ctx, `
		select `+eventColumns+`
		from events
		where source_type in ('slack', 'gmail', 'gdrive', 'notion') and not tombstoned and (ingested_at, id) > ($1, $2)
		order by ingested_at, id
		limit $3
	`, wmTime, wmID, p.scanLimit())
	if err != nil {
		return nil, fmt.Errorf("scan events: %w", err)
	}
	return collectEvents(rows)
}

func collectEvents(rows pgx.Rows) ([]scannedEvent, error) {
	defer rows.Close()
	var events []scannedEvent
	for rows.Next() {
		var ev scannedEvent
		var acl string
		if err := rows.Scan(&ev.ID, &ev.TenantID, &ev.SourceType, &ev.ThreadKey, &ev.OccurredAt, &ev.IngestedAt, &acl, &ev.PayloadRef, &ev.AuthorSourceRef); err != nil {
			return nil, fmt.Errorf("scan event row: %w", err)
		}
		ev.ACL = []byte(acl)
		events = append(events, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scan events: %w", err)
	}
	return events, nil
}

func (p *Pipeline) processTenant(ctx context.Context, model string, tenantID uuid.UUID, scanned []scannedEvent) (int, int, error) {
	events, err := p.fetchContext(ctx, tenantID, scanned)
	if err != nil {
		return 0, 0, err
	}

	messages, deadLettered, err := p.toMessages(ctx, tenantID, events)
	if err != nil {
		return 0, 0, err
	}
	chunks := chunkBySource(messages)

	scannedSet := map[uuid.UUID]bool{}
	for _, ev := range scanned {
		scannedSet[ev.ID] = true
	}
	candidateWindows := map[string]bool{}
	chunkWindows := map[string]bool{}
	affectedIDs := map[uuid.UUID]bool{}
	for id := range scannedSet {
		affectedIDs[id] = true
	}
	for _, c := range chunks {
		chunkWindows[c.WindowKey] = true
		for _, member := range c.EventIDs {
			if scannedSet[member] {
				candidateWindows[c.WindowKey] = true
				for _, m := range c.EventIDs {
					affectedIDs[m] = true
				}
				break
			}
		}
	}

	priorWindows, err := p.priorWindows(ctx, model, tenantID, keys(affectedIDs))
	if err != nil {
		return 0, 0, err
	}

	var orphaned []string
	for key := range priorWindows {
		if !chunkWindows[key] {
			orphaned = append(orphaned, key)
		}
	}
	if len(orphaned) > 0 {
		extra, err := p.fetchWindows(ctx, tenantID, orphaned)
		if err != nil {
			return 0, 0, err
		}
		byID := map[uuid.UUID]scannedEvent{}
		for _, ev := range events {
			byID[ev.ID] = ev
		}
		for _, ev := range extra {
			byID[ev.ID] = ev
		}
		merged := make([]scannedEvent, 0, len(byID))
		for _, ev := range byID {
			merged = append(merged, ev)
		}
		messages, _, err = p.toMessages(ctx, tenantID, merged)
		if err != nil {
			return 0, 0, err
		}
		chunks = chunkBySource(messages)
	}

	windows := map[string]bool{}
	for key := range candidateWindows {
		windows[key] = true
	}
	for key := range priorWindows {
		windows[key] = true
	}

	toWrite := make([]chunker.Chunk, 0, len(chunks))
	for _, c := range chunks {
		if windows[c.WindowKey] {
			toWrite = append(toWrite, c)
		}
	}

	embedded, embedDead, err := p.embed(ctx, tenantID, toWrite)
	if err != nil {
		return 0, 0, err
	}
	deadLettered += embedDead

	if err := p.replaceWindows(ctx, model, tenantID, windows, embedded); err != nil {
		return 0, 0, err
	}
	return len(embedded), deadLettered, nil
}

func chunkBySource(messages []chunker.Message) []chunker.Chunk {
	var chat []chunker.Message
	var email []chunker.Message
	var documents []chunker.Message
	for _, message := range messages {
		switch message.SourceType {
		case "gmail":
			email = append(email, message)
		case "gdrive", "notion":
			documents = append(documents, message)
		default:
			chat = append(chat, message)
		}
	}
	chunks := chunker.ChunkChat(chat)
	chunks = append(chunks, chunker.ChunkEmail(email)...)
	chunks = append(chunks, chunker.ChunkDocument(documents)...)
	return chunks
}

func keys(set map[uuid.UUID]bool) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	return out
}

func (p *Pipeline) fetchWindows(ctx context.Context, tenantID uuid.UUID, windowKeys []string) ([]scannedEvent, error) {
	var all []scannedEvent
	for _, key := range windowKeys {
		eventID, isSingleEvent := strings.CutPrefix(key, "email:")
		if !isSingleEvent {
			eventID, isSingleEvent = strings.CutPrefix(key, "doc:")
		}
		if isSingleEvent {
			rows, err := p.Pool.Query(ctx, `
				select `+eventColumns+`
				from events
				where tenant_id = $1 and id = $2::uuid and not tombstoned
			`, tenantID, eventID)
			if err != nil {
				return nil, fmt.Errorf("fetch window %q: %w", key, err)
			}
			events, err := collectEvents(rows)
			if err != nil {
				return nil, err
			}
			all = append(all, events...)
			continue
		}
		channel, hourRaw, isHourWindow := strings.Cut(key, "@")
		var rows pgx.Rows
		var err error
		if isHourWindow {
			hour, parseErr := time.Parse("2006-01-02T15", hourRaw)
			if parseErr != nil {
				return nil, fmt.Errorf("parse window key %q: %w", key, parseErr)
			}
			rows, err = p.Pool.Query(ctx, `
				select `+eventColumns+`
				from events
				where source_type in ('slack', 'gmail', 'gdrive', 'notion') and not tombstoned and tenant_id = $1
				  and thread_key like $2
				  and occurred_at >= $3 and occurred_at < $4
			`, tenantID, channel+":%", hour, hour.Add(time.Hour))
		} else {
			rows, err = p.Pool.Query(ctx, `
				select `+eventColumns+`
				from events
				where source_type in ('slack', 'gmail', 'gdrive', 'notion') and not tombstoned and tenant_id = $1 and thread_key = $2
			`, tenantID, key)
		}
		if err != nil {
			return nil, fmt.Errorf("fetch window %q: %w", key, err)
		}
		events, err := collectEvents(rows)
		if err != nil {
			return nil, err
		}
		all = append(all, events...)
	}
	return all, nil
}

func (p *Pipeline) fetchContext(ctx context.Context, tenantID uuid.UUID, scanned []scannedEvent) ([]scannedEvent, error) {
	threadKeys := map[string]bool{}
	for _, ev := range scanned {
		threadKeys[ev.ThreadKey] = true
	}
	keys := make([]string, 0, len(threadKeys))
	for key := range threadKeys {
		keys = append(keys, key)
	}

	rows, err := p.Pool.Query(ctx, `
		select `+eventColumns+`
		from events
		where source_type in ('slack', 'gmail', 'gdrive', 'notion') and not tombstoned and tenant_id = $1 and thread_key = any($2)
	`, tenantID, keys)
	if err != nil {
		return nil, fmt.Errorf("fetch threads: %w", err)
	}
	events, err := collectEvents(rows)
	if err != nil {
		return nil, err
	}

	byID := map[uuid.UUID]scannedEvent{}
	for _, ev := range events {
		byID[ev.ID] = ev
	}

	threadSizes := map[string]int{}
	for _, ev := range events {
		threadSizes[ev.ThreadKey]++
	}
	type hourWindow struct {
		channel string
		hour    time.Time
	}
	seen := map[hourWindow]bool{}
	for _, ev := range events {
		if ev.SourceType != "slack" || threadSizes[ev.ThreadKey] >= 2 {
			continue
		}
		w := hourWindow{channelOf(ev.ThreadKey), ev.OccurredAt.UTC().Truncate(time.Hour)}
		if seen[w] {
			continue
		}
		seen[w] = true
		peerRows, err := p.Pool.Query(ctx, `
			select `+eventColumns+`
			from events
			where source_type in ('slack', 'gmail', 'gdrive', 'notion') and not tombstoned and tenant_id = $1
			  and thread_key like $2
			  and occurred_at >= $3 and occurred_at < $4
		`, tenantID, w.channel+":%", w.hour, w.hour.Add(time.Hour))
		if err != nil {
			return nil, fmt.Errorf("fetch hour window: %w", err)
		}
		peers, err := collectEvents(peerRows)
		if err != nil {
			return nil, err
		}
		for _, peer := range peers {
			byID[peer.ID] = peer
		}
	}

	all := make([]scannedEvent, 0, len(byID))
	for _, ev := range byID {
		all = append(all, ev)
	}
	return all, nil
}

func channelOf(threadKey string) string {
	channel, _, found := strings.Cut(threadKey, ":")
	if !found {
		return threadKey
	}
	return channel
}

func (p *Pipeline) toMessages(ctx context.Context, tenantID uuid.UUID, events []scannedEvent) ([]chunker.Message, int, error) {
	messages := make([]chunker.Message, 0, len(events))
	deadLettered := 0
	for _, ev := range events {
		content, err := p.Payloads.Get(ctx, ev.PayloadRef)
		if err != nil {
			if dlqErr := p.deadLetter(ctx, tenantID, ev.ID, nil, "payload: "+err.Error()); dlqErr != nil {
				return nil, 0, dlqErr
			}
			deadLettered++
			continue
		}
		var body payloadBody
		if err := json.Unmarshal(content, &body); err != nil {
			if dlqErr := p.deadLetter(ctx, tenantID, ev.ID, nil, "payload_decode: "+err.Error()); dlqErr != nil {
				return nil, 0, dlqErr
			}
			deadLettered++
			continue
		}
		messages = append(messages, chunker.Message{
			EventID:    ev.ID,
			OccurredAt: ev.OccurredAt,
			SourceType: ev.SourceType,
			ThreadKey:  ev.ThreadKey,
			Author:     strings.TrimPrefix(ev.AuthorSourceRef, "slack:"),
			Text:       body.Body,
			ACL:        ev.ACL,
		})
	}
	return messages, deadLettered, nil
}

func (p *Pipeline) priorWindows(ctx context.Context, model string, tenantID uuid.UUID, affectedIDs []uuid.UUID) (map[string]bool, error) {
	windows := map[string]bool{}
	rows, err := p.Pool.Query(ctx, `
		select distinct window_key from event_chunks
		where tenant_id = $1 and embedding_model = $2 and member_event_ids && $3
	`, tenantID, model, affectedIDs)
	if err != nil {
		return nil, fmt.Errorf("prior windows: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("prior windows: %w", err)
		}
		windows[key] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("prior windows: %w", err)
	}
	return windows, nil
}

type embeddedChunk struct {
	chunker.Chunk
	vector []float32
}

func (p *Pipeline) embed(ctx context.Context, tenantID uuid.UUID, chunks []chunker.Chunk) ([]embeddedChunk, int, error) {
	var embedded []embeddedChunk
	deadLettered := 0
	size := p.batchSize()

	for start := 0; start < len(chunks); start += size {
		end := min(start+size, len(chunks))
		batch := chunks[start:end]
		contents := make([]string, len(batch))
		for i, c := range batch {
			contents[i] = c.Content
		}

		vectors, err := p.Provider.Embed(ctx, contents)
		if err != nil {
			for _, c := range batch {
				vector, singleErr := p.Provider.Embed(ctx, []string{c.Content})
				if singleErr != nil {
					if dlqErr := p.deadLetter(ctx, tenantID, c.AnchorEventID, &c.ChunkIndex, "embed: "+singleErr.Error()); dlqErr != nil {
						return nil, 0, dlqErr
					}
					deadLettered++
					continue
				}
				embedded = append(embedded, embeddedChunk{Chunk: c, vector: vector[0]})
			}
			continue
		}
		for i, c := range batch {
			embedded = append(embedded, embeddedChunk{Chunk: c, vector: vectors[i]})
		}
	}
	return embedded, deadLettered, nil
}

func (p *Pipeline) replaceWindows(ctx context.Context, model string, tenantID uuid.UUID, windows map[string]bool, chunks []embeddedChunk) error {
	keys := make([]string, 0, len(windows))
	for key := range windows {
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return nil
	}

	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin replace: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		delete from event_chunks
		where tenant_id = $1 and embedding_model = $2 and window_key = any($3)
	`, tenantID, model, keys); err != nil {
		return fmt.Errorf("delete windows: %w", err)
	}

	for _, c := range chunks {
		memberIDs := make([]uuid.UUID, len(c.EventIDs))
		copy(memberIDs, c.EventIDs)
		if _, err := tx.Exec(ctx, `
			insert into event_chunks (
				tenant_id, event_id, event_occurred_at, chunk_index, content,
				embedding, embedding_model, acl, token_count, window_key, member_event_ids, source_type
			)
			values ($1, $2, $3, $4, $5, $6::extensions.vector, $7, $8::jsonb, $9, $10, $11, $12)
		`, tenantID, c.AnchorEventID, c.AnchorOccurredAt, c.ChunkIndex, c.Content,
			vectorLiteral(c.vector), model, string(c.ACL), c.TokenEstimate, c.WindowKey, memberIDs, c.SourceType,
		); err != nil {
			return fmt.Errorf("insert chunk %s/%d: %w", c.WindowKey, c.ChunkIndex, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit replace: %w", err)
	}
	return nil
}

func (p *Pipeline) deadLetter(ctx context.Context, tenantID, eventID uuid.UUID, chunkIndex *int, reason string) error {
	_, err := p.Pool.Exec(ctx, `
		insert into embedding_dlq (tenant_id, event_id, chunk_index, reason)
		values ($1, $2, $3, $4)
	`, tenantID, eventID, chunkIndex, reason)
	if err != nil {
		return fmt.Errorf("embedding dead letter: %w", err)
	}
	return nil
}

func vectorLiteral(vector []float32) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, v := range vector {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(v), 'f', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}
