package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/elionaz/panal/internal/apierr"
	"github.com/elionaz/panal/internal/client"
)

func TestFormatError(t *testing.T) {
	if got := formatError(errors.New("plain")); got != "plain" {
		t.Fatalf("plain: %v", got)
	}
	remote := &client.RemoteError{Msg: "1 live session: x (cat); use --force to stop them", Code: apierr.LiveSessions,
		Data: json.RawMessage(`{"count":1}`)}
	got, ok := formatError(fmt.Errorf("wrapped: %w", remote)).(string)
	if !ok {
		t.Fatalf("not a string: %v", got)
	}
	var ce codedError
	if err := json.Unmarshal([]byte(got), &ce); err != nil || ce.Code != apierr.LiveSessions ||
		ce.Message != "wrapped: 1 live session: x (cat); use --force to stop them" || string(ce.Data) != `{"count":1}` {
		t.Fatalf("coded: %s (%v)", got, err)
	}
	local := apierr.New(apierr.MergeNothing, map[string]any{"branch": "b"}, "nothing to merge")
	if got := formatError(local).(string); got != `{"message":"nothing to merge","code":"merge_nothing","data":{"branch":"b"}}` {
		t.Fatalf("local: %s", got)
	}
}
