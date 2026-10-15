package claude_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/martintrifunov/orkestar/internal/agent"
	"github.com/martintrifunov/orkestar/internal/agent/claude"
)

func TestAdapterCapabilities(t *testing.T) {
	t.Parallel()

	capabilities := claude.New("").Capabilities()
	if capabilities.Name != "claude-code" {
		t.Fatalf("unexpected adapter name: %q", capabilities.Name)
	}
	if !capabilities.SupportsInteractive || !capabilities.SupportsPrompt || !capabilities.SupportsInterrupt {
		t.Fatalf("unexpected capabilities: %#v", capabilities)
	}
	if capabilities.SupportsManaged || !capabilities.SupportsResume {
		t.Fatalf("unexpected capabilities: %#v", capabilities)
	}
}

func TestAdapterRejectsManagedMode(t *testing.T) {
	t.Parallel()

	adapter := claude.New(fixtureExecutable(t))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := adapter.Launch(ctx, agent.LaunchOptions{Mode: agent.ModeManaged}); err == nil {
		t.Fatal("expected managed mode to be rejected")
	}
}

func TestAdapterAcceptsResume(t *testing.T) {
	t.Parallel()

	executable := fixtureExecutable(t)
	adapter := claude.New(executable)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	session, err := adapter.Launch(ctx, agent.LaunchOptions{Mode: agent.ModeInteractive, ResumeSessionID: "prior"})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if session.NativeSessionID() != "prior" {
		t.Fatal("resume identity not retained")
	}
	if args := fixtureArguments(t, executable); len(args) != 2 || args[0] != "--resume" || args[1] != "prior" {
		t.Fatalf("unexpected launch arguments: %#v", args)
	}
}

// An opening prompt must reach Claude Code as its initial prompt, not be
// typed at the terminal later: in a directory it has not been trusted in,
// the CLI shows a trust dialog first, and a late Enter there selects the
// default answer, which is to exit. The prompt also carries a separator so
// one that begins with a dash does not parse as a flag.
func TestAdapterTakesTheOpeningPromptAtLaunch(t *testing.T) {
	t.Parallel()

	executable := fixtureExecutable(t)
	adapter := claude.New(executable)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	session, err := adapter.Launch(ctx, agent.LaunchOptions{
		Mode:          agent.ModeInteractive,
		Directory:     t.TempDir(),
		OpeningPrompt: "-fix the flaky test",
	})
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	defer session.Close()

	promptSession, ok := session.(agent.OpeningPromptSession)
	if !ok {
		t.Fatal("expected the interactive session to report on the opening prompt")
	}
	if !promptSession.OpeningPromptHandled() {
		t.Fatal("a launch with an opening prompt should report it submitted")
	}
	if args := fixtureArguments(t, executable); len(args) != 2 || args[0] != "--" || args[1] != "-fix the flaky test" {
		t.Fatalf("unexpected launch arguments: %#v", args)
	}
}

func TestAdapterLaunchPromptAndInterrupt(t *testing.T) {
	t.Parallel()

	adapter := claude.New(fixtureExecutable(t))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	session, err := adapter.Launch(ctx, agent.LaunchOptions{
		Mode:      agent.ModeInteractive,
		Directory: t.TempDir(),
		Columns:   80,
		Rows:      24,
	})
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	defer session.Close()

	waitForState(t, session, agent.StateReady)

	if err := session.Prompt(ctx, "hello"); err != nil {
		t.Fatalf("prompt: %v", err)
	}
	if err := session.Interrupt(ctx); err != nil {
		t.Fatalf("interrupt: %v", err)
	}

	processSession, ok := session.(agent.ProcessSession)
	if !ok {
		t.Fatal("expected session to implement agent.ProcessSession")
	}
	if err := processSession.Process().Close(); err != nil {
		t.Fatalf("close process: %v", err)
	}

	waitForState(t, session, agent.StateStopped)
}

func waitForState(t *testing.T, session agent.Session, want agent.State) {
	t.Helper()

	deadline := time.After(5 * time.Second)
	var seen []string
	for {
		select {
		case event, ok := <-session.Events():
			if !ok {
				t.Fatalf("events channel closed before reaching state %q, saw %v", want, seen)
			}
			seen = append(seen, fmt.Sprintf("%v(%s)", event.State, event.Reason))
			if event.State == want {
				return
			}
		case <-deadline:
			// Report what did arrive: a wrong terminal state and no state at
			// all need different fixes, and this only fails under CI load.
			t.Fatalf("timed out waiting for state %q; saw %v, session reports %v", want, seen, session.State())
		}
	}
}

// fixtureExecutable writes a small shell script that behaves enough like an
// interactive CLI (reads stdin, ignores it, exits when its PTY closes) to
// exercise the adapter without depending on a real Claude Code install. It
// records its own arguments to "<path>.args" so a test can check how the
// adapter invoked it.
func fixtureExecutable(t *testing.T) string {
	t.Helper()

	directory := t.TempDir()
	path := filepath.Join(directory, "claude")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$0.args\"\ncat >/dev/null\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fixture executable: %v", err)
	}
	return path
}

// fixtureArguments returns the arguments the fixture executable was launched
// with, waiting briefly because the script writes them as it starts.
func fixtureArguments(t *testing.T, executable string) []string {
	t.Helper()

	path := executable + ".args"
	deadline := time.Now().Add(3 * time.Second)
	for {
		data, err := os.ReadFile(path)
		if err == nil {
			lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
			if len(lines) == 1 && lines[0] == "" {
				return nil
			}
			return lines
		}
		if time.Now().After(deadline) {
			t.Fatalf("fixture executable never recorded its arguments: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
