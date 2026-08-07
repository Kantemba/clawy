package security

// Guard provides a unified security guard for the agent.
// It combines prompt injection defense, filesystem sandboxing,
// SSRF protection, and privacy redaction.
type Guard struct {
	Prompt   *PromptInjectionDefense
	FS       *FilesystemSandbox
	SSRF     *SSRFProtection
	Redactor *Redactor
}

// NewGuard creates a new security guard with sensible defaults.
func NewGuard(opts ...GuardOption) *Guard {
	g := &Guard{
		Prompt:   NewPromptInjectionDefense(),
		SSRF:     NewSSRFProtection(),
		Redactor: NewRedactor(),
	}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

// GuardOption configures a Guard.
type GuardOption func(*Guard)

// WithPromptDefense configures prompt injection defense.
func WithPromptDefense(d *PromptInjectionDefense) GuardOption {
	return func(g *Guard) { g.Prompt = d }
}

// WithFilesystemSandbox configures the filesystem sandbox.
func WithFilesystemSandbox(s *FilesystemSandbox) GuardOption {
	return func(g *Guard) { g.FS = s }
}

// WithSSRFProtection configures SSRF protection.
func WithSSRFProtection(s *SSRFProtection) GuardOption {
	return func(g *Guard) { g.SSRF = s }
}

// WithRedactor configures the redactor.
func WithRedactor(r *Redactor) GuardOption {
	return func(g *Guard) { g.Redactor = r }
}

// DefaultGuard creates a guard with secure defaults.
func DefaultGuard(workspaceDir string) *Guard {
	return NewGuard(
		WithPromptDefense(NewPromptInjectionDefense()),
		WithFilesystemSandbox(NewFilesystemSandbox(workspaceDir)),
		WithSSRFProtection(NewSSRFProtection()),
		WithRedactor(NewRedactor()),
	)
}
