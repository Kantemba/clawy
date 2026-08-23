package evolution

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Kantemba/clawy/pkg/providers"
)

// identitySectionBegin / identitySectionEnd delimit the auto-curated facts
// within SOUL.md and USER.md. Keeping them bounded between stable markers
// makes curation idempotent: repeated turns only ever append net-new facts,
// and the file never grows unbounded or accumulates duplicate headings.
const (
	identitySectionBegin = "<!-- identity:begin -->"
	identitySectionEnd   = "<!-- identity:end -->"
)

// maxFactsPerTurn caps how many facts a single turn may append to each file.
// Combined with the curator cool-down this keeps curation cheap and bounded.
const maxFactsPerTurn = 3

// defaultCuratorCooldown is the minimum interval between two curations of the
// same workspace. Mirrors Hermes's "periodically nudges itself" behaviour so
// the agent does not pay an LLM round-trip on every single turn.
const defaultCuratorCooldown = 5 * time.Minute

// IdentityCurateInput describes a finished turn worth curating identity from.
// Only successful turns should be curated (the runtime enforces this).
type IdentityCurateInput struct {
	Workspace    string
	TurnID       string
	Success      bool
	UserMessage  string
	FinalContent string
}

// IdentityCurateResult reports what the curator did.
type IdentityCurateResult struct {
	Updated   bool
	SoulFacts []string
	UserFacts []string
	Error     string
}

// IdentityCurator produces targeted additions to an agent's identity layer
// (SOUL.md / USER.md) from turn feedback.
//
// It is the online counterpart to the batch cold path: rather than waiting for
// a clustering cycle to rediscover who the user is and what the agent stands
// for, the curator writes worth-keeping facts straight into the identity files
// at the end of each curated turn.
type IdentityCurator interface {
	Curate(ctx context.Context, input IdentityCurateInput) (IdentityCurateResult, error)
}

// LLMIdentityCurator uses an LLM to extract durable identity facts and appends
// them to SOUL.md and USER.md (creating the files if absent). Deduplicates
// against existing content and rate-limits via a file-mtime cool-down.
//
// With no provider or model configured it is a no-op (no identity can be
// extracted without an LLM), so agents on $10 hardware simply never curate.
type LLMIdentityCurator struct {
	workspace string
	provider  providers.LLMProvider
	model     string
	cooldown  time.Duration
	now       func() time.Time
}

// NewLLMIdentityCurator returns a curator bound to the given workspace.
func NewLLMIdentityCurator(workspace string, provider providers.LLMProvider, model string) *LLMIdentityCurator {
	return &LLMIdentityCurator{
		workspace: workspace,
		provider:  provider,
		model:     strings.TrimSpace(model),
		cooldown:  defaultCuratorCooldown,
		now:       time.Now,
	}
}

// Curate extracts and persists identity facts. It is a no-op (returning a zero
// result and nil error) when there is no provider/model, when the turn was not
// successful, when the cool-down has not elapsed, or when the LLM yields nothing
// worth persisting.
func (r *LLMIdentityCurator) Curate(ctx context.Context, input IdentityCurateInput) (IdentityCurateResult, error) {
	if r == nil || r.provider == nil || r.model == "" {
		return IdentityCurateResult{}, nil
	}
	if !input.Success {
		return IdentityCurateResult{}, nil
	}
	if strings.TrimSpace(input.FinalContent) == "" {
		return IdentityCurateResult{}, nil
	}

	// Cool-down: skip if SOUL.md was touched very recently. A missing file
	// (first curation) is exempt.
	soulPath := soulPath(r.workspace)
	if fi, err := os.Stat(soulPath); err == nil {
		if !fi.ModTime().IsZero() && r.now().Sub(fi.ModTime()) < r.cooldown {
			return IdentityCurateResult{}, nil
		}
	}

	facts, err := r.extractFacts(ctx, input)
	if err != nil || len(facts.Soul)+len(facts.User) == 0 {
		return IdentityCurateResult{}, nil
	}

	result := IdentityCurateResult{}
	soulText, soulChanged := applyFacts(readFileOrEmpty(soulPath), facts.Soul, maxFactsPerTurn)
	if soulChanged {
		if err := os.WriteFile(soulPath, []byte(soulText), 0o644); err == nil {
			result.Updated = true
			result.SoulFacts = facts.Soul
		}
	}
	userPath := userPath(r.workspace)
	userText, userChanged := applyFacts(readFileOrEmpty(userPath), facts.User, maxFactsPerTurn)
	if userChanged {
		if err := os.WriteFile(userPath, []byte(userText), 0o644); err == nil {
			result.Updated = true
			result.UserFacts = facts.User
		}
	}
	return result, nil
}

// identityFacts is the JSON shape the LLM is asked to return.
type identityFacts struct {
	Soul []string `json:"soul,omitempty"`
	User []string `json:"user,omitempty"`
}

// extractFacts asks the LLM for curated SOUL.md and USER.md facts.
func (r *LLMIdentityCurator) extractFacts(ctx context.Context, input IdentityCurateInput) (identityFacts, error) {
	callCtx, cancel := withLLMCallTimeout(ctx, llmDraftGenerationTimeout)
	defer cancel()

	soulBody := readFileOrEmpty(soulPath(r.workspace))
	userBody := readFileOrEmpty(userPath(r.workspace))
	resp, err := r.provider.Chat(callCtx, []providers.Message{
		{
			Role: "system",
			Content: "You are a disciplined identity curator for a coding agent. From the turn below, " +
				"extract DURABLE facts worth persisting into the agent's identity profile (SOUL.md) and " +
				"the user's profile (USER.md). Output exactly one JSON object with keys \"soul\" and " +
				"\"user\", each a list of at most 3 concise bullet strings without markdown formatting " +
				"(no leading \"- \" or \"* \", no \"#\" headings). If nothing is worth persisting, return " +
				"empty lists. Do not repeat facts likely already present in the existing profiles. " +
				"Do not wrap the JSON in markdown fences.",
		},
		{
			Role: "user",
			Content: "## user message\n" + strings.TrimSpace(input.UserMessage) +
				"\n\n## final output\n" + summarizeText(input.FinalContent, 2000) +
				"\n\n## existing SOUL.md\n" + truncateForPrompt(soulBody, 1500) +
				"\n\n## existing USER.md\n" + truncateForPrompt(userBody, 1500),
		},
	}, nil, r.model, map[string]any{"temperature": 0.3})
	if err != nil || resp == nil {
		return identityFacts{}, err
	}
	facts, _ := parseIdentityFacts(resp.Content)
	return facts, nil
}

// parseIdentityFacts tolerantly parses the LLM's JSON response, dropping any
// markdown fence lines (```` ``` ```` or ```` ```lang ````) so a fenced JSON
// object is still parsed. Non-JSON input yields ok=false.
func parseIdentityFacts(content string) (identityFacts, bool) {
	body := strings.TrimSpace(content)
	if body == "" {
		return identityFacts{}, false
	}
	cleaned := make([]string, 0, 16)
	for _, ln := range strings.Split(body, "\n") {
		l := strings.TrimSpace(ln)
		if l == "" {
			continue
		}
		if strings.HasPrefix(l, "```") {
			continue
		}
		cleaned = append(cleaned, ln)
	}
	body = strings.TrimSpace(strings.Join(cleaned, "\n"))
	var out identityFacts
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		return identityFacts{}, false
	}
	return out, true
}

// applyFacts merges new facts into the identity section of fileContent,
// returning the rewritten content and whether anything was added. Duplicate
// facts (case-insensitive substring of an existing line) are skipped, and the
// total number of newly-appended facts is capped at maxAppend.
func applyFacts(fileContent string, newFacts []string, maxAppend int) (string, bool) {
	clean := make([]string, 0, len(newFacts))
	for _, f := range newFacts {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		clean = append(clean, f)
	}
	existing := sectionFacts(fileContent)
	existing = append(existing, sectionLines(fileContent)...)

	added := make([]string, 0, len(clean))
	for _, f := range clean {
		if isFactPresent(existing, f) {
			continue
		}
		if len(added) >= maxAppend {
			break
		}
		added = append(added, f)
	}
	if len(added) == 0 {
		return fileContent, false
	}

	updated := append(sectionFacts(fileContent), added...)
	section := identitySectionBegin + "\n" + joinFacts(updated) + identitySectionEnd
	return spliceSection(fileContent, section), true
}

// sectionFacts returns the curated facts currently between the identity markers.
func sectionFacts(fileContent string) []string {
	begin := strings.Index(fileContent, identitySectionBegin)
	end := strings.Index(fileContent, identitySectionEnd)
	if begin < 0 || end < 0 || end <= begin {
		return nil
	}
	between := fileContent[begin+len(identitySectionBegin) : end]
	return nonEmptyLines(between)
}

// sectionLines returns every non-empty line in fileContent (used as the
// deduplication corpus beyond the curated section).
func sectionLines(fileContent string) []string {
	return nonEmptyLines(fileContent)
}

func nonEmptyLines(s string) []string {
	out := []string{}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

// spliceSection replaces an existing identity section in fileContent, or
// appends a fresh one (with a trailing newline) when absent.
func spliceSection(fileContent, section string) string {
	begin := strings.Index(fileContent, identitySectionBegin)
	end := strings.Index(fileContent, identitySectionEnd)
	if begin >= 0 && end > begin {
		end += len(identitySectionEnd)
		return fileContent[:begin] + section + fileContent[end:]
	}
	trimmed := strings.TrimRight(fileContent, "\n")
	if trimmed == "" {
		return section + "\n"
	}
	return trimmed + "\n\n" + section + "\n"
}

// joinFacts formats curated facts as bullet list lines.
func joinFacts(facts []string) string {
	b := strings.Builder{}
	for _, f := range facts {
		b.WriteString("- ")
		b.WriteString(f)
		b.WriteString("\n")
	}
	return b.String()
}

// normFact lowercases, collapses whitespace, strips a leading list marker
// (e.g. "- " from a bullet line) and trims trailing sentence punctuation so
// that "Be helpful." and "Be helpful and kind." dedup against each other on
// their shared stem.
func normFact(s string) string {
	fields := strings.Fields(s)
	for len(fields) > 0 && isListMarker(fields[0]) {
		fields = fields[1:]
	}
	if len(fields) == 0 {
		return ""
	}
	joined := strings.ToLower(strings.Join(fields, " "))
	return strings.TrimRight(joined, ".,;:!?")
}

// isListMarker reports whether tok is a markdown list bullet ("-", "*", "+")
// or an ordered-list number ("1.").
func isListMarker(tok string) bool {
	switch tok {
	case "-", "*", "+":
		return true
	}
	if len(tok) >= 2 && tok[len(tok)-1] == '.' {
		return allDigits(tok[:len(tok)-1])
	}
	return false
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// isFactPresent reports whether candidate already appears as (or is
// redundant with) one of the existing lines. It matches in both directions on
// normalized text so that an exact repeat ("Be helpful.") and a near-duplicate
// superset ("Be helpful and kind.") are both treated as already present.
func isFactPresent(existing []string, candidate string) bool {
	hay := normFact(candidate)
	if hay == "" {
		return true
	}
	for _, e := range existing {
		e = normFact(e)
		if e == "" {
			continue
		}
		if strings.Contains(e, hay) || strings.Contains(hay, e) {
			return true
		}
	}
	return false
}

func truncateForPrompt(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

func soulPath(workspace string) string {
	return filepath.Join(workspace, "SOUL.md")
}

func userPath(workspace string) string {
	return filepath.Join(workspace, "USER.md")
}

func readFileOrEmpty(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}
