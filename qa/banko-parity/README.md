# Banko parity

celle replaces the date helpers of Banko (`deviationist/banko`, archived
2026-11-03): `/api/utils/days-until-date`, `bank-holiday-calc`,
`next-payment-date` and `days-until-payment-date`. This folder runs the same
questions through Banko's real code and through celle, and groups every
difference by cause.

## Running it

Banko's repository is private, so this needs access to it (or another copy of
Banko's code). The findings below stand on their own.

1. Clone Banko and install its locked dependencies (no database, no secrets):

   ```sh
   git clone git@github.com:deviationist/banko.git /tmp/banko && cd /tmp/banko
   composer install --no-scripts --no-dev --ignore-platform-reqs
   cat > .env <<'ENV'
   APP_ENV=local
   APP_KEY=base64:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=
   APP_TIMEZONE=UTC
   CACHE_STORE=array
   SESSION_DRIVER=array
   DB_CONNECTION=sqlite
   DB_DATABASE=:memory:
   ENV
   ```

   `APP_ENV=testing` fails (Livewire then wants Mockery).

2. `make parity BANKO=/tmp/banko` from the celle checkout. It exits non-zero if
   any difference has no known cause.

The harness pins Banko's clock with `Carbon::setTestNow` and lifts its API rate
limit. The sweep covers every day 2020–2035, every payday 1–31 and `last` in
every month 2020–2035 (with and without a country), and day counts around five
"now" instants, two of them either side of Oslo midnight.

## Result (2026-11-17)

20,337 cases. Every difference is one of the causes below, all of them Banko
bugs or intended changes. Outside these, celle and Banko agree on every day:
rickar/cal's Norway holidays match Banko's own Easter-based list for 2020–2035.

| Cause | Banko | celle | Why celle differs |
|---|---|---|---|
| **Julaften** | Treats 24 Dec as a banking day (its `includeChristmasEve` flag was never turned on) | Closed | Norges Bank's settlement system (NBO) is closed on julaften |
| **Nyttårsaften** | Treats 31 Dec as closed | Banking day | NBO is open on 31 Dec. Only the Swedish and Danish central banks close (Bits NICS calendar) |
| **Day past the month's end** | `payment_day=31` in February gives 3 March (Carbon overflow) | Last day of the month, then rolled | A payday never leaves its month |
| **Passed payday sign** | `days-until-payment-date` returns a passed payday as positive (Carbon 3's `diffInDays` is already signed; Banko negated it again) | Negative | |
| **Time zone** | "Today" is the UTC date (`config/app.php` hard-codes UTC), so from midnight to 01:00/02:00 Oslo time every count is off by one | Today in `Europe/Oslo`, or `?tz=` | |

Sources for the bank days:
[Norges Bank: Oppgjørsdager](https://www.norges-bank.no/tema/Norges-Banks-oppgjorssystem/Oppgjorsdager/),
[Bits: NICS-kalender 2025](https://bits.no/document/nics-kalender-2025).

## Other Banko shortcomings (not covered by the sweep)

- **Rate limit**: the `api` route group had Laravel's default 60 requests a
  minute per IP. Google Sheets fetches `IMPORTDATA` from Google's servers, so
  a sheet with many Banko cells could get HTTP 429 (the first sweep hit it).
  celle has no per-IP limit.
- **No auth on the utils endpoints**: anyone could call them. celle requires a
  token.
- **Only rolls backwards**: celle adds `roll=next`.
- **Year and month required for paydays**: a sheet had to build the current
  month into the URL. celle's `/v1/payday` without year/month gives the next
  payday.
- **Dates only as text**: a date cell concatenated into a URL becomes a serial
  number (46380), which Banko couldn't read. celle accepts serials and
  `DD.MM.YYYY`.
- **`format=` took a PHP date pattern** (`d.m.Y`). celle's `date_format` takes
  spreadsheet-style tokens (`DD.MM.YYYY`).
- **Errors as JSON** in a cell. celle answers `ERROR: …` as text with `raw`
  (IMPORTDATA still shows its own error for non-200 answers).
- **Palmesøndag, 1. påskedag and 1. pinsedag** were in Banko's list. They are
  Sundays, so they never change a banking-day answer; celle's holiday list
  (rickar/cal) leaves them out.
