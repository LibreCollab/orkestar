package codex

import (
	"context"
	"fmt"

	"github.com/martintrifunov/orkestar/internal/agent"
	"github.com/martintrifunov/orkestar/internal/agent/hooks"
	"github.com/martintrifunov/orkestar/internal/agent/ptysession"
)

type Adapter struct{ executable string }

func New(executable string) *Adapter {
	if executable == "" {
		executable = "codex"
	}
	return &Adapter{executable}
}
func (a *Adapter) Capabilities() agent.Capabilities {
	return agent.Capabilities{Name: "codex", SupportsInteractive: true, SupportsPrompt: true, SupportsInterrupt: true, SupportsResume: true}
}
func (a *Adapter) Launch(ctx context.Context, o agent.LaunchOptions) (agent.Session, error) {
	if o.Mode != agent.ModeInteractive {
		return nil, fmt.Errorf("codex supports interactive mode only")
	}
	if o.HookCommand != "" {
		o.Arguments = append(o.Arguments, hooks.Codex(o.HookCommand)...)
	}
	if o.ResumeSessionID != "" {
		o.Arguments = append(o.Arguments, "resume", o.ResumeSessionID)
	}
	// An opening prompt is handed to the CLI as its initial prompt, for the
	// same reason as the claude adapter: in a directory that has not been
	// trusted yet, Codex shows a trust dialog before its hooks or input box
	// exist, and typing at it after the fact would answer it by accident.
	// The separator keeps a prompt that begins with a dash or names a
	// subcommand from being parsed as one.
	if o.OpeningPrompt != "" {
		o.Arguments = append(o.Arguments, "--", o.OpeningPrompt)
	}
	session, err := ptysession.Launch("codex", a.executable, o)
	if err != nil {
		return nil, err
	}
	if o.OpeningPrompt != "" {
		session.MarkOpeningPromptHandled()
	}
	return session, nil
}
