package client

import (
	"strings"
	"testing"

	"github.com/elionaz/agenterm/internal/apierr"
	"github.com/elionaz/agenterm/internal/protocol"
)

func TestRemoteErrorUnknownMethod(t *testing.T) {
	for name, resp := range map[string]protocol.Response{
		"coded":        {Error: `unknown method "worktree.pr" (daemon 0.4.0)`, ErrorCode: apierr.UnknownMethod},
		"before 0.4.0": {Error: `unknown method "worktree.pr"`},
	} {
		e := remoteError("worktree.pr", resp)
		if e.Code != apierr.UnknownMethod || !strings.Contains(e.Msg, "does not support worktree.pr: it is older than this agenterm") ||
			string(e.Data) != `{"method":"worktree.pr"}` {
			t.Errorf("%s: %+v", name, e)
		}
	}
	e := remoteError("session.kill", protocol.Response{Error: "session not found: x", ErrorCode: ""})
	if e.Code != "" || e.Msg != "session not found: x" {
		t.Errorf("other errors are kept: %+v", e)
	}
}
