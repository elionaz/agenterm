package daemon

import (
	"os/exec"
	"reflect"
	"syscall"
	"testing"

	"github.com/elionaz/agenterm/internal/profile"
	"github.com/elionaz/agenterm/internal/protocol"
	"github.com/elionaz/agenterm/internal/sessionstore"
)

func TestRestartArgs(t *testing.T) {
	claudeInj := []string{"--settings", "/h/claude-settings.json"}
	codexInj := []string{"-c", "notify=[...]"}
	claude := profile.Profile{ID: "claude", Args: []string{"--model", "opus"}, SetupHooks: "claude"}
	codex := profile.Profile{ID: "codex", Args: []string{"-m", "o3"}, SetupHooks: "codex"}
	opencode := profile.Profile{ID: "opencode", ResumeArgs: []string{"--continue"}}
	custom := profile.Profile{ID: "mine", Args: []string{"--fast"}, ResumeArgs: []string{"--resume", "last"}}
	shell := profile.Profile{ID: "shell"}
	ref := func(agent, id string) *sessionstore.Resume { return &sessionstore.Resume{Agent: agent, SessionID: id} }

	cases := []struct {
		name string
		prof profile.Profile
		inj  []string
		r    *sessionstore.Resume
		want []string
		mode string
	}{
		{"claude with id", claude, claudeInj, ref("claude", "u-1"),
			[]string{"--settings", "/h/claude-settings.json", "--resume", "u-1", "--model", "opus"}, protocol.ResumeAgent},
		{"claude without id", claude, claudeInj, nil,
			[]string{"--settings", "/h/claude-settings.json", "--model", "opus"}, protocol.ResumeFresh},
		{"claude id from another agent", claude, claudeInj, ref("codex", "t-1"),
			[]string{"--settings", "/h/claude-settings.json", "--model", "opus"}, protocol.ResumeFresh},
		{"codex with thread id", codex, codexInj, ref("codex", "t-9"),
			[]string{"resume", "-c", "notify=[...]", "-m", "o3", "t-9"}, protocol.ResumeAgent},
		{"codex without id", codex, codexInj, nil, []string{"-c", "notify=[...]", "-m", "o3"}, protocol.ResumeFresh},
		{"opencode resume_args", opencode, nil, nil, []string{"--continue"}, protocol.ResumeArgs},
		{"custom resume_args", custom, nil, nil, []string{"--fast", "--resume", "last"}, protocol.ResumeArgs},
		{"shell", shell, nil, nil, []string{}, protocol.ResumeFresh},
	}
	for _, c := range cases {
		got, mode := restartArgs(c.prof, c.inj, c.r)
		if !reflect.DeepEqual(got, c.want) || mode != c.mode {
			t.Errorf("%s: got %q (%s), want %q (%s)", c.name, got, mode, c.want, c.mode)
		}
	}
	// The inputs are not modified.
	if !reflect.DeepEqual(claudeInj, []string{"--settings", "/h/claude-settings.json"}) || len(claude.Args) != 2 {
		t.Fatal("inputs modified")
	}
}

func TestProcessAliveRejectsStrangers(t *testing.T) {
	if processAlive(0, "sh") || processAlive(-1, "sh") {
		t.Fatal("non-positive pid")
	}
	// This test process is not a process-group leader running "zzz-agent".
	if processAlive(1, "zzz-agent-that-does-not-exist") {
		t.Fatal("pid 1 matched")
	}
	// A group leader running the command is alive (no double spawn).
	cmd := exec.Command("/bin/sleep", "30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cmd.Process.Kill(); cmd.Wait() }()
	if !processAlive(cmd.Process.Pid, "/bin/sleep") {
		t.Fatal("running group leader not detected")
	}
	if processAlive(cmd.Process.Pid, "claude") {
		t.Fatal("other command matched")
	}
}
