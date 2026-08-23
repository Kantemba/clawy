package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Kantemba/clawy/pkg/session"
	"github.com/Kantemba/clawy/pkg/utils"
)

// SessionSearchTool gives the agent cross-session recall over past
// conversations, Hermes-style: the curated memory stores hold distilled
// knowledge, while session_search retrieves verbatim history on demand.
// Ranking uses the same BM25 engine as Clawy's other search surfaces.
type SessionSearchTool struct {
	store session.SessionStore
}

// NewSessionSearchTool builds a session recall tool over a session store.
func NewSessionSearchTool(store session.SessionStore) *SessionSearchTool {
	return &SessionSearchTool{store: store}
}

func (t *SessionSearchTool) Name() string {
	return "session_search"
}

func (t *SessionSearchTool) Description() string {
	return "Search past conversation sessions when you need something that happened before but is not in " +
		"your curated memory yet. Returns ranked snippets from previous chats with their session keys and " +
		"message indices. Use for recalling earlier decisions, links, names, or context; distill anything " +
		"you want to keep long-term into the `memory` tool."
}

func (t *SessionSearchTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "What to look for in past conversations",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Maximum number of message hits to return (default 8, max 25)",
			},
		},
		"required": []string{"query"},
	}
}

// sessionDoc is one indexable unit: a single stored message.
type sessionDoc struct {
	sessionKey string
	index      int
	role       string
	content    string
}

func (d sessionDoc) searchText() string {
	return d.content
}

func (t *SessionSearchTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	if t.store == nil {
		return ErrorResult("session store is unavailable")
	}

	query, _ := args["query"].(string)
	query = strings.TrimSpace(query)
	if query == "" {
		return ErrorResult("query is required")
	}
	limit := 8
	if v, ok := args["limit"].(float64); ok && v > 0 {
		limit = int(v)
	}
	if limit > 25 {
		limit = 25
	}

	var docs []sessionDoc
	for _, key := range t.store.ListSessions() {
		// SessionStore.GetHistory is fire-and-forget: it returns an empty
		// slice for missing/unreadable sessions, which we simply skip.
		msgs := t.store.GetHistory(key)
		for i, msg := range msgs {
			content := strings.TrimSpace(msg.Content)
			if content == "" {
				continue
			}
			docs = append(docs, sessionDoc{
				sessionKey: key,
				index:      i,
				role:       msg.Role,
				content:    content,
			})
		}
	}

	if len(docs) == 0 {
		return SilentResult("No past conversation data available to search.")
	}

	engine := utils.NewBM25Engine(docs, sessionDoc.searchText)
	hits := engine.Search(query, limit)
	if len(hits) == 0 {
		return SilentResult(fmt.Sprintf("No matches for %q in %d indexed messages.", query, len(docs)))
	}

	sort.Slice(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })

	var sb strings.Builder
	fmt.Fprintf(&sb, "Found %d match(es) for %q:\n\n", len(hits), query)
	for _, hit := range hits {
		snippet := snippetAround(hit.Document.content, query, 320)
		fmt.Fprintf(&sb, "[%s | msg #%d | %s]\n%s\n\n", hit.Document.sessionKey, hit.Document.index, hit.Document.role, snippet)
	}
	sb.WriteString("Distill anything worth keeping into the `memory` tool; this history may eventually be compacted away.")
	return SilentResult(sb.String())
}

// snippetAround returns a window of content centered on the first query-term
// match so results stay readable instead of dumping whole messages.
func snippetAround(content, query string, maxLen int) string {
	content = strings.Join(strings.Fields(content), " ")
	if len(content) <= maxLen {
		return content
	}

	center := -1
	for _, term := range strings.Fields(strings.ToLower(query)) {
		if idx := strings.Index(strings.ToLower(content), term); idx >= 0 {
			center = idx
			break
		}
	}
	if center < 0 {
		center = 0
	}

	start := center - maxLen/3
	if start < 0 {
		start = 0
	}
	end := start + maxLen
	if end > len(content) {
		end = len(content)
		start = end - maxLen
		if start < 0 {
			start = 0
		}
	}

	prefix := ""
	if start > 0 {
		prefix = "…"
	}
	suffix := ""
	if end < len(content) {
		suffix = "…"
	}
	return prefix + content[start:end] + suffix
}
