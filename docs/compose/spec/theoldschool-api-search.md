---
feature: theoldschool-api-search
status: in-progress
updated: 2026-11-04
source: feat/theoldschool-api-search
---

# The Old School via Cardigann

## Report

## [S1] Problem

The Old School (theoldschool.cc, French private tracker) exposes a UNIT3D-style
JSON API (`api_token` → `Authorization: Bearer`, `GET /api/torrents`,
`GET /api/torrents/filter`). It was unclear whether gowlarr handles this API
shape natively or needs a Prowlarr round-trip, and two field chains of the
corpus definition `theoldschool-api.yml` (synced from Prowlarr/Indexers)
silently produced empty values: the `files[0].name` single-file title and the
`append " +00:00"` → `dateparse … zzz` date chain. A third gap was found
during implementation: Prowlarr-style `leechers` fields never reached
`release.Peers`.

## [S2] Design

### S2.1 Activation is configuration-only

`gowlarr defs sync` → `gowlarr indexer add theoldschool-api --setting
apikey=… [--id tos]`. The definition resolves by fuzzy match (score ≥ 60);
`apikey` is a required text setting (TTY prompt when omitted); `type: private`
maps to protocol `torrent` (`internal/service/indexer.go` default branch);
the instance ID defaults to the definition domain. The existing search/download
flow routes Cardigann definitions through `BuildConfiguredProviders` →
`buildCardigannProvider` → `cardigannadapter.Provider` with no code changes.
The def's `login` block is a harmless no-op (GET without auth; body error
selectors never match a JSON 401). Enablement is documented in README
(« Private tracker example (The Old School) »).

### S2.2 Regression coverage in cardigann-go against a verbatim corpus fixture

- `internal/defstest/testdata/theoldschool-api.yml` — verbatim copy of
  `Prowlarr/Indexers/definitions/v11/theoldschool-api.yml`; automatically
  covered by `TestCorpusFixture_{ParseAndProtocol,BeforeChainParses,PathsRender}`
  via `fixtureIDs()`. Precedent: `aither-api.yml` and 30+ committed corpus
  fixtures. gowlarr's « never redistributed » policy is repo-scoped: no corpus
  YAML is added to gowlarr; its adapter stays covered generically by
  `internal/cardigannadapter/localjson_test.go`.
- `engine/theoldschool_fixture_test.go` — end-to-end fixture test against an
  httptest UNIT3D-shaped server: login pre-check on `/api/torrents`, search on
  `/api/torrents/filter`; asserts Bearer auth, request inputs
  (`name`, `sortField`, `sortDirection`, `perPage`, `seasonNumber`,
  `episodeNumber`, no `free[]` by default, no empty ID params), and response
  extraction (single-file title from `files[0].name`, multi-file title from
  `name`, seeders, leechers→peers, size, native category, absolutized
  details/download, infohash, `PublishDate` from the append+00:00 date chain,
  protocol torrent). A second case pins `free[]=100` when `freeleech=true`.

### S2.3 Three engine fixes (test-first)

Discovered by T2 failing exactly on these, then fixed:

1. **Bracket indexes** (`selector/json.go`): `normalizeBracketIndexes` rewrites
   digit-only `key[N]` → `key.N` before gjson (JSON `Find`/`Attr` only; other
   syntax passes through). Json.NET-style corpus selectors (74 defs) now
   resolve.
2. **dateparse colon offsets** (`template/filters/filters.go` `dateParse`):
   after .NET→Go layout translation, also try the swapped zone variant
   (`-0700` ↔ `-07:00`) so `+00:00` inputs parse against `zzz` layouts (104
   defs). `netTokens` itself is unchanged — colon-less inputs keep parsing on
   the primary layout (existing `TestDateParse_NetLayout` untouched).
3. **leechers → Peers** (`engine/extract.go` `fieldsToRelease`): accept the
   Prowlarr-style `leechers` field name as fallback for the canonical
   `peers`. Found during T2 implementation (not in the original plan);
   without it `release.Peers` stays 0 for TOS and aither-style defs.

Known non-blockers observed while writing the test (unchanged, by design):
JSON `case` keys `False`/`True` don't match lowercase `false`/`true`
(case-sensitive; Prowlarr is case-insensitive) so `_internal`/`_featured`
helpers defer — cosmetic for TOS (description prefix; volume factors are
parsed but not propagated anyway). `mapCategories` is a no-op for TOS
(category names, not numeric ids).

## [S3] Out of Scope

- Upload API (`POST /api/torrents/upload`) — gowlarr is search/download only.
- TVDBID plumbing (`search.Query` has no field; gowlarr's Torznab caps don't
  advertise `tvdbid`).
- Native TOS client; Torznab→Prowlarr proxy client.
- Propagating `downloadvolumefactor` / `uploadvolumefactor` /
  `minimumseedtime` / `grabs` through `release.Release`.
- Category-name → Newznab-id mapping polish for TOS-style defs.
- Case-insensitive JSON `case` matching.
- Tagging cardigann-go / bumping gowlarr's `v0.8.0` pin (finish decision).

## Tasks

- [x] T1: corpus fixture `internal/defstest/testdata/theoldschool-api.yml` — acceptance: `go test ./internal/defstest/` passes with `theoldschool-api` subtests (covers: S2.2)
- [x] T2: engine fixture test `engine/theoldschool_fixture_test.go` — acceptance: request/response assertions green after fixes; freeleech case pins `free[]=100` (covers: S2.2)
- [x] T3: bracket-index normalization in `selector/json.go` + unit test — acceptance: `Find("files[0].name")` resolves; selector suite green (covers: S2.3.1)
- [x] T4: dateparse colon-offset variant in `template/filters/filters.go` + unit test — acceptance: `+00:00` input parses; existing colon-less tests green (covers: S2.3.2)
- [x] T4b: `leechers` → `Peers` fallback in `engine/extract.go` — acceptance: TOS test peers=3 (covers: S2.3.3)
- [x] T5: README « Private tracker example (The Old School) » — acceptance: four commands with exact CLI syntax; no corpus YAML in gowlarr (covers: S2.1)
- [x] T6: this spec — acceptance: S1/S2/S3 + tasks match delivery (covers: S2)
- [ ] T7: live verification with the user's real API token — acceptance: `indexer test tos` OK and non-empty `search --json` results (depends: T1–T6)

## Journey log

- Plan validated by two independent neutral reviewers (technical accuracy +
  archi/process), both `APPROVE WITH MINOR FIXES`; all required fixes
  integrated before implementation (replace-directive trap for live testing,
  T2 contingency, WIP inventory, token handling via TTY prompt).
- T2 failed on **three** gaps, not the planned two: `leechers` was never
  mapped to `Peers` (no aliasing anywhere in the engine). Added T4b as the
  minimal fix consistent with the planned assertion.
- JSON `case` keys `False`/`True` vs lowercase JSON booleans: observed
  case-sensitivity mismatch with Prowlarr semantics; left unchanged
  (out of scope, cosmetic — see S2.3).
