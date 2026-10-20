// Package claude implements the Claude Code interactive agent adapter. It
// launches the installed `claude` executable inside a PTY and adapts it to
// the internal/agent contracts; there is no managed/SDK mode yet. An opening
// prompt is passed to the CLI as its initial prompt rather than typed into
// the terminal after a startup signal, so a session blocked on a directory
// trust dialog is left alone instead of being typed at.
package claude

import (
	"context"
	"fmt"

	"github.com/LibreCollab/orkestar/internal/agent"
	"github.com/LibreCollab/orkestar/internal/agent/hooks"
	"github.com/LibreCollab/orkestar/internal/agent/ptysession"
)

const defaultExecutable = "claude"

// Adapter launches Claude Code interactively in a PTY.
type Adapter struct {
	executable string
}

// New returns an Adapter that runs the given executable. An empty
// executable defaults to "claude" resolved from PATH. Tests can point this
// at a fixture script instead of a real Claude Code installation.
func New(executable string) *Adapter {
	if executable == "" {
		executable = defaultExecutable
	}
	return &Adapter{executable: executable}
}

func (a *Adapter) Capabilities() agent.Capabilities {
	return agent.Capabilities{
		Name:                "claude-code",
		SupportsInteractive: true,
		SupportsManaged:     false,
		SupportsPrompt:      true,
		SupportsInterrupt:   true,
		SupportsResume:      true,
	}
}

func (a *Adapter) Launch(ctx context.Context, options agent.LaunchOptions) (agent.Session, error) {
	if options.Mode != agent.ModeInteractive {
		return nil, fmt.Errorf("claude adapter: mode %q is not supported", options.Mode)
	}
	if options.ResumeSessionID != "" {
		options.Arguments = append(options.Arguments, "--resume", options.ResumeSessionID)
	}
	if options.HookCommand != "" {
		options.Arguments = append(options.Arguments, "--settings", hooks.Claude(options.HookCommand))
	}
	// An opening prompt is handed to the CLI as its initial prompt. Typing it
	// in after a startup signal instead would go wrong wherever Claude Code
	// does not reach its input box on its own — in a directory it has not been
	// trusted in, it shows a trust dialog first, and a late Enter there picks
	// its default answer, which is to exit. Passed at launch, the prompt waits
	// behind the dialog and is submitted once someone trusts the folder. The
	// separator keeps a prompt that begins with a dash from parsing as a flag.
	if options.OpeningPrompt != "" {
		options.Arguments = append(options.Arguments, "--", options.OpeningPrompt)
	}
	session, err := ptysession.Launch("claude", a.executable, options)
	if err != nil {
		return nil, fmt.Errorf("claude adapter: %w", err)
	}
	if options.OpeningPrompt != "" {
		session.MarkOpeningPromptHandled()
	}
	return session, nil
}
