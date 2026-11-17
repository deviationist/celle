package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
)

// PlainTexter is implemented by response bodies that have a bare-value form.
type PlainTexter interface {
	PlainText() string
}

// TextBody is a body that is only a string.
type TextBody struct {
	Value string `json:"value"`
}

func (b TextBody) PlainText() string { return b.Value }

type TextOutput struct{ Body TextBody }

// textFormat writes PlainTexter values as they are and errors as
// "ERROR: <detail>", so a spreadsheet cell shows something readable.
var textFormat = huma.Format{
	Marshal: func(w io.Writer, v any) error {
		switch b := v.(type) {
		case PlainTexter:
			_, err := io.WriteString(w, b.PlainText())
			return err
		case *huma.ErrorModel:
			msg := b.Detail
			for _, e := range b.Errors {
				msg += "; " + e.Error()
			}
			_, err := fmt.Fprintf(w, "ERROR: %s", msg)
			return err
		default:
			return json.NewEncoder(w).Encode(v)
		}
	},
	Unmarshal: func(data []byte, v any) error {
		return fmt.Errorf("text/plain request bodies are not supported")
	},
}

// withRaw turns ?raw (Banko's flag) into Accept: text/plain, because
// IMPORTDATA cannot set headers.
func withRaw(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if q := r.URL.Query(); q.Has("raw") && q.Get("raw") != "false" && q.Get("raw") != "0" {
			r.Header.Set("Accept", "text/plain")
		}
		next.ServeHTTP(w, r)
	})
}

// withCommonHeaders allows browser callers (an Excel add-in runs in a
// webview) and stops caches from keeping answers that depend on today.
func withCommonHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Access-Control-Allow-Origin", "*")
		if r.Method == http.MethodOptions {
			h.Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Accept, Content-Type")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/v1/") || r.URL.Path == "/healthz" {
			h.Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}
