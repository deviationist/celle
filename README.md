# celle

Date helpers for spreadsheets: day counts, banking days, holidays and paydays,
served as a small HTTP API for Google Sheets and Microsoft Excel. It replaces
the non-bank features of Banko (see `qa/banko-parity/README.md` for what
changed and why).

Go + [Huma](https://huma.rocks/) (OpenAPI 3.1 at `/openapi.json`, docs at
`/docs`). Holiday rules come from [rickar/cal](https://github.com/rickar/cal),
plus the Norwegian bank-closing day julaften.

## Using it from a sheet

Every `/v1/` endpoint needs a token. `IMPORTDATA` can't send headers, so pass
it as `token=`; other clients can send `Authorization: Bearer <token>`. Add
`raw` to get the bare value as text instead of JSON.

```
=IMPORTDATA("https://celle.example.com/v1/days-until-payday?raw&day=25&country=NO&token=…")
```

Norwegian-locale Sheets use `;` between arguments. Concatenating a date cell
works as is (it arrives as a serial number):
`"…/v1/days-until?raw&date=" & A1 & "&token=…"`.

## Endpoints

| Endpoint | Parameters | Answer (`raw`) |
|---|---|---|
| `GET /v1/days-until` | `date` | Calendar days from today, negative if passed |
| `GET /v1/banking-days-until` | `date`, `country` | Banking days after today up to and including `date` |
| `GET /v1/banking-day` | `date`, `country`, `roll=previous\|next` | The date, or the nearest banking day before/after it |
| `GET /v1/payday` | `day` (1–31 or `last`), optional `year`+`month`, `country`, `roll` | Without year/month: the next payday on or after today. A day past the month's end is the last day. With `country`, rolled to a banking day |
| `GET /v1/days-until-payday` | same as `/v1/payday` | Calendar days to that payday |
| `GET /v1/holidays` | `country`, `year` | One `date,name` line per holiday (JSON adds `kind`: `public` or `bank`) |
| `GET /v1/countries` | | Supported countries |
| `GET /healthz` | | `ok`, no token |

Common parameters:
- `date`: `YYYY-MM-DD`, `DD.MM.YYYY` or a spreadsheet serial number.
- `date_format`: output pattern with `YYYY`, `YY`, `MM`, `M`, `DD`, `D`.
- `tz`: IANA zone that decides "today" (default `CELLE_DEFAULT_TZ`, `Europe/Oslo`).
- `country`: ISO code (`NO`); `norway`/`norge` are accepted for Banko URLs.

Supported countries: Norway. Adding one is an entry in
`internal/calendar/calendar.go` plus its bank-only closing days.

## Development

```sh
cp .env.example .env    # set CELLE_TOKENS (openssl rand -hex 24)
make run                # :8080
make check              # gofmt, vet, tests: the gate
make docker             # build the image
```

## Deploying

Run the container (`compose.yaml`, or `docker build` and your own runner)
behind a TLS reverse proxy. For `IMPORTDATA` it has to be reachable from the
internet, because Google fetches the URL from its servers; the token is the
only auth. Make sure the proxy doesn't log query strings, since the token is
in them.

Configuration is environment only: `CELLE_TOKENS` (required, comma-separated,
16+ characters each, no expiry), `CELLE_DEFAULT_TZ`, `CELLE_ADDR`, and for
compose `CELLE_BIND`/`CELLE_PORT`.
