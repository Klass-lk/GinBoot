package parity

import (
	"encoding/base64"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func textResp(body string) Response {
	return Response{Status: 200, Header: http.Header{"Content-Type": {"text/plain"}}, Body: []byte(body)}
}

func jwtOf(header, payload string) string {
	enc := base64.RawURLEncoding.EncodeToString
	return enc([]byte(header)) + "." + enc([]byte(payload)) + ".c2lnbmF0dXJl"
}

func TestDecodeJWT(t *testing.T) {
	ref := jwtOf(`{"alg":"HS512"}`, `{"roles":["STUDENT"],"sub":"u1","iat":100,"exp":200}`)
	sameClaimsLater := jwtOf(`{"alg":"HS512"}`, `{"sub":"u1","roles":["STUDENT"],"iat":150,"exp":250}`)
	otherUser := jwtOf(`{"alg":"HS512"}`, `{"roles":["STUDENT"],"sub":"u2","iat":100,"exp":200}`)
	rules := Rules{Decode: "jwt", Ignore: []string{"$.payload.iat", "$.payload.exp"}}

	diffs, err := Compare(textResp(ref), textResp(sameClaimsLater), rules)
	require.NoError(t, err)
	assert.Empty(t, diffs, "only the ignored times differ")

	diffs, err = Compare(textResp(ref), textResp(otherUser), rules)
	require.NoError(t, err)
	assert.Equal(t, map[string]DiffKind{"$.payload.sub": DiffValue}, kinds(diffs))

	diffs, err = Compare(textResp("not a token"), textResp("not a token"), rules)
	require.NoError(t, err)
	assert.Empty(t, diffs, "undecodable bodies are compared as they are")
}

func TestDecodeExec(t *testing.T) {
	rules := Rules{Decode: "exec:tr -d 0-9"}
	diffs, err := Compare(textResp("report 2026-01-01"), textResp("report 2027-12-31"), rules)
	require.NoError(t, err)
	assert.Empty(t, diffs, "the command's output is compared")

	diffs, err = Compare(textResp("report a"), textResp("report b"), rules)
	require.NoError(t, err)
	assert.Equal(t, map[string]DiffKind{"body": DiffBody}, kinds(diffs))
}

func TestDecodeRejectsUnknown(t *testing.T) {
	_, err := Compare(textResp("a"), textResp("a"), Rules{Decode: "base64"})
	assert.Error(t, err)
}

func TestDecodeJWTRefusesMalformedTokens(t *testing.T) {
	enc := base64.RawURLEncoding.EncodeToString
	for name, body := range map[string]string{
		"two parts":         "a.b",
		"header not base64": "!!!." + enc([]byte(`{}`)) + ".sig",
		"payload not JSON":  enc([]byte(`{}`)) + "." + enc([]byte(`not json`)) + ".sig",
		"header not JSON":   enc([]byte(`{`)) + "." + enc([]byte(`{}`)) + ".sig",
	} {
		_, ok := decodeJWT([]byte(body))
		assert.False(t, ok, name)
	}
	out, ok := decodeJWT([]byte(" " + jwtOf(`{"alg":"HS512"}`, `{"sub":"u1"}`) + "\n"))
	require.True(t, ok, "surrounding space is trimmed")
	assert.JSONEq(t, `{"header":{"alg":"HS512"},"payload":{"sub":"u1"}}`, string(out))
}

func TestMergeDecode(t *testing.T) {
	assert.Equal(t, "jwt", Rules{Decode: "jwt"}.Merge(Rules{}).Decode, "kept when other has none")
	assert.Equal(t, "exec:cat", Rules{Decode: "jwt"}.Merge(Rules{Decode: "exec:cat"}).Decode, "other's wins")
}

func TestCompileRejectsBadPatterns(t *testing.T) {
	for name, rules := range map[string]Rules{
		"ignore":        {Ignore: []string{"no-dollar"}},
		"nullIsMissing": {NullIsMissing: []string{"no-dollar"}},
		"ignoreQuery":   {IgnoreQuery: []string{"no-dollar"}},
		"unordered":     {Unordered: map[string]string{"no-dollar": ""}},
	} {
		_, err := rules.compile()
		assert.Error(t, err, name)
	}
}
