package parity

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os/exec"
	"strings"
	"time"
)

// decode applies the Decode rule to a body; see Rules.Decode.
func (c *compiledRules) decode(body []byte) []byte {
	switch {
	case c.Decode == "jwt":
		if out, ok := decodeJWT(body); ok {
			return out
		}
	case strings.HasPrefix(c.Decode, "exec:"):
		if out, ok := decodeExec(strings.TrimPrefix(c.Decode, "exec:"), body); ok {
			return out
		}
	}
	return body
}

// decodeJWT returns {"header": ..., "payload": ...} for a compact JWS.
func decodeJWT(body []byte) ([]byte, bool) {
	parts := strings.Split(strings.TrimSpace(string(body)), ".")
	if len(parts) != 3 {
		return nil, false
	}
	var out struct {
		Header  json.RawMessage `json:"header"`
		Payload json.RawMessage `json:"payload"`
	}
	for i, dst := range []*json.RawMessage{&out.Header, &out.Payload} {
		raw, err := base64.RawURLEncoding.DecodeString(parts[i])
		if err != nil || !json.Valid(raw) {
			return nil, false
		}
		*dst = raw
	}
	b, err := json.Marshal(out)
	return b, err == nil
}

// decodeExec pipes body through a shell command, giving up after a minute.
func decodeExec(command string, body []byte) ([]byte, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Stdin = bytes.NewReader(body)
	out, err := cmd.Output()
	return out, err == nil
}
