package chunker

import (
	"bytes"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

const chatTokenBudget = 800

type Message struct {
	EventID    uuid.UUID
	OccurredAt time.Time
	ThreadKey  string
	Author     string
	Text       string
	ACL        []byte
}

type Chunk struct {
	AnchorEventID    uuid.UUID
	AnchorOccurredAt time.Time
	WindowKey        string
	ChunkIndex       int
	Content          string
	TokenEstimate    int
	ACL              []byte
	EventIDs         []uuid.UUID
}

func EstimateTokens(s string) int {
	return len([]rune(s))/4 + 1
}

func ChunkChat(messages []Message) []Chunk {
	byThread := map[string][]Message{}
	for _, m := range messages {
		byThread[m.ThreadKey] = append(byThread[m.ThreadKey], m)
	}

	windows := map[string][]Message{}
	threaded := map[string]bool{}
	for key, group := range byThread {
		if len(group) >= 2 {
			windows[key] = group
			threaded[key] = true
			continue
		}
		single := group[0]
		windowKey := channelOf(single.ThreadKey) + "@" + single.OccurredAt.UTC().Format("2006-01-02T15")
		windows[windowKey] = append(windows[windowKey], single)
	}

	windowKeys := make([]string, 0, len(windows))
	for key := range windows {
		windowKeys = append(windowKeys, key)
	}
	sort.Strings(windowKeys)

	var chunks []Chunk
	for _, key := range windowKeys {
		group := windows[key]
		sort.Slice(group, func(i, j int) bool {
			if group[i].OccurredAt.Equal(group[j].OccurredAt) {
				return group[i].EventID.String() < group[j].EventID.String()
			}
			return group[i].OccurredAt.Before(group[j].OccurredAt)
		})
		chunks = append(chunks, chunkWindow(key, group, threaded[key])...)
	}
	return chunks
}

func channelOf(threadKey string) string {
	channel, _, found := strings.Cut(threadKey, ":")
	if !found {
		return threadKey
	}
	return channel
}

func chunkWindow(windowKey string, messages []Message, withOverlap bool) []Chunk {
	var chunks []Chunk
	index := 0
	for _, run := range splitByACL(messages) {
		for _, members := range packRun(run, withOverlap) {
			chunks = append(chunks, buildChunk(windowKey, members, index))
			index++
		}
	}
	return chunks
}

func splitByACL(messages []Message) [][]Message {
	var runs [][]Message
	for _, m := range messages {
		last := len(runs) - 1
		if last >= 0 && bytes.Equal(runs[last][0].ACL, m.ACL) {
			runs[last] = append(runs[last], m)
			continue
		}
		runs = append(runs, []Message{m})
	}
	return runs
}

func packRun(run []Message, withOverlap bool) [][]Message {
	var packed [][]Message
	var current []Message
	currentTokens := 0

	for _, m := range run {
		tokens := EstimateTokens(line(m))
		if len(current) > 0 && currentTokens+tokens > chatTokenBudget {
			packed = append(packed, current)
			if withOverlap {
				overlap := current[len(current)-1]
				current = []Message{overlap}
				currentTokens = EstimateTokens(line(overlap))
			} else {
				current = nil
				currentTokens = 0
			}
		}
		current = append(current, m)
		currentTokens += tokens
	}
	if len(current) > 0 {
		packed = append(packed, current)
	}
	return packed
}

func buildChunk(windowKey string, members []Message, index int) Chunk {
	lines := make([]string, len(members))
	eventIDs := make([]uuid.UUID, len(members))
	for i, m := range members {
		lines[i] = line(m)
		eventIDs[i] = m.EventID
	}
	content := strings.Join(lines, "\n")
	anchor := members[len(members)-1]
	return Chunk{
		AnchorEventID:    anchor.EventID,
		AnchorOccurredAt: anchor.OccurredAt,
		WindowKey:        windowKey,
		ChunkIndex:       index,
		Content:          content,
		TokenEstimate:    EstimateTokens(content),
		ACL:              members[0].ACL,
		EventIDs:         eventIDs,
	}
}

func line(m Message) string {
	if m.Author == "" {
		return m.Text
	}
	return m.Author + ": " + m.Text
}
