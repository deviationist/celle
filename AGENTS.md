# celle: agent notes

Date-helper API for spreadsheets (Google Sheets `IMPORTDATA`, later an Excel
add-in and a Sheets add-on). Go + Huma, no database. Replaces Banko's
`/api/utils` endpoints; Banko itself is retired and archived.

## Layout

- `cmd/celle`: main, env config, `-health` probe for the container healthcheck.
- `internal/api`: Huma operations (`api.go`), token auth (`auth.go`), the
  `text/plain` format and `?raw` handling (`text.go`).
- `internal/dates`: date parsing, day counts, rolling, paydays. Pure functions.
- `internal/calendar`: banking calendars per country on top of rickar/cal.
- `qa/banko-parity`: sweep against Banko's real code. Findings in its README.

## Rules

- **`make check` is the gate** (gofmt, vet, tests).
- **Every response must be usable from `IMPORTDATA`**: a `raw` form that is one
  bare value (or CSV lines), no headers needed. New body types implement
  `PlainText()`.
- **Bank days come from authoritative calendars**, not memory: Norges Bank's
  *Oppgjørsdager* and Bits' NICS calendar for Norway. rickar/cal gives public
  holidays only; bank-only days (julaften) are added in `calendar.go`.
  `cal.Holiday.Clone` ignores `Month`/`Day` overrides: build such days directly.
- **"Today" is never the server's UTC date**: always through `Config.Now` and a
  time zone (`?tz=`, default `CELLE_DEFAULT_TZ`).
- **Tokens never expire** and live only in the env file (`CELLE_TOKENS`). The
  process refuses to start without one. Never log query strings: the token is in
  them.
- Responses carry `Cache-Control: no-store` (answers depend on today).

## Deploy

The repo is generic: nothing here names a particular host, domain or network.
Keep it that way; instance-specific deploy notes belong with whoever runs the
instance, not in this repo.

What any deployment needs:
- The container (`compose.yaml`) with `CELLE_TOKENS` in an env file beside it.
- A TLS reverse proxy in front. For `IMPORTDATA` the endpoint must be reachable
  from the public internet (Google fetches it from its own servers), so the
  token is the only auth.
- The proxy must not log query strings: the token is in them.
