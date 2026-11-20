package main

import (
	"encoding/json"

	"github.com/elionaz/panal/internal/apierr"
)

// codedError is how a coded daemon refusal reaches the frontend. Wails
// turns any non-string error value into "[object Object]", so it travels
// as this JSON in the rejection's message; errorMessage() in api.ts
// decodes it and renders the code in the UI language.
type codedError struct {
	Message string          `json:"message"`
	Code    string          `json:"code"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// formatError is the Wails ErrorFormatter: plain errors stay their text,
// coded ones (apierr.Coded, e.g. client.RemoteError) become JSON.
func formatError(err error) any {
	code, data, ok := apierr.Code(err)
	if !ok {
		return err.Error()
	}
	ce := codedError{Message: err.Error(), Code: code}
	switch d := data.(type) {
	case json.RawMessage:
		ce.Data = d
	case nil:
	default:
		ce.Data, _ = json.Marshal(d)
	}
	b, merr := json.Marshal(ce)
	if merr != nil {
		return err.Error()
	}
	return string(b)
}
