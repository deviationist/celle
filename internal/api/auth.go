package api

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
)

// metaPublic marks an operation that needs no token.
const metaPublic = "public"

// tokenAuth accepts a token from "Authorization: Bearer <token>" or from the
// ?token= query parameter (IMPORTDATA cannot send headers). Tokens never
// expire; they are revoked by removing them from CELLE_TOKENS.
func tokenAuth(a huma.API, tokens []string) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		if op := ctx.Operation(); op != nil && op.Metadata[metaPublic] == true {
			next(ctx)
			return
		}
		got := ""
		if h := ctx.Header("Authorization"); strings.HasPrefix(h, "Bearer ") {
			got = strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
		} else {
			got = ctx.Query("token")
		}
		if got == "" {
			_ = huma.WriteErr(a, ctx, http.StatusUnauthorized, "missing token: add ?token=… or an Authorization: Bearer header")
			return
		}
		for _, t := range tokens {
			if subtle.ConstantTimeCompare([]byte(got), []byte(t)) == 1 {
				next(ctx)
				return
			}
		}
		_ = huma.WriteErr(a, ctx, http.StatusUnauthorized, "invalid token")
	}
}

// ParseTokens splits a comma- or whitespace-separated token list and drops
// empty entries.
func ParseTokens(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\n' || r == '\t'
	})
}

func securitySchemes() map[string]*huma.SecurityScheme {
	return map[string]*huma.SecurityScheme{
		"bearer": {Type: "http", Scheme: "bearer", Description: "Authorization: Bearer <token>"},
		"query":  {Type: "apiKey", In: "query", Name: "token", Description: "?token=<token>, for IMPORTDATA"},
	}
}
