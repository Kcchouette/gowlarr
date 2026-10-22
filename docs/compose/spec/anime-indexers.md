---
feature: anime-indexers
status: in-progress
updated: 2026-10-22
branch: feat/anime-indexers
commits:
---

# Anime Indexers (local JSON definition + native Newznab)

## Report

## [S1] Problem

Three anime indexer integrations are requested. Two of them (an NZB site and
an anime NZB mirror) expose standard Newznab APIs that the existing native
Newznab provider already supports — one of them even without an API key — but
nothing documents or verifies those concrete configurations. The third one
exposes a proprietary JSON API (name/season/episode params, `date` as Unix
epoch seconds, results under `items`) that no existing path covers: Cardigann
definitions parsed responses as HTML by default, and cardigann-go has neither
a template variable for the Newznab `t` parameter (14 synced Prowlarr
definitions render `t` empty, silently degrading `tvsearch`) nor any
epoch-to-date filter (a `date` field would parse as RFC3339-fail and silently
zero `PublishDate`).

## [S2] Design

- **Local Cardigann definition** `definitions-local/aninzb.yml` (real file in
  the main checkout — `definitions-local/` is gitignored as a separate repo;
  the definition content is versioned as a Go inline constant in the tests):
  `type: usenet`, `links: https://api.aninzb.moe/`, single category mapping
  `5070` (TV/Anime), modes `search [q]` / `tv-search [q, season, ep]`, no
  login, no `allowrawsearch` (the engine's empty-search gate covers the
  API's 404-on-empty-root; season/episode-only searches pass the gate). One
  GET path `/` with `response: {type: json}` (mandatory — the default is
  HTML), `Accept: application/json`, inputs `name`/`season`/`episode`
  (empty season/episode are dropped by `filterInputs`), rows `$.items`
  (gjson, per-item relative field selectors), fields: `title ← filename`,
  `download ← nzb` (`optional: true` — items may lack it), `size ← size`,
  `date ← date` + filter `unixdate`, `category = 5070`, `details` built
  conditionally from an optional `_anidb` helper (nullable AniDB id never
  yields a truncated URL).
- **cardigann-go upstream fixes** (repo owned by this project's author):
  1. `engine.Query.Type` field, populated into `rctx.Query["Type"]` with a
     default of `"search"` when empty — repairs the three corpus `t:`
     templates (verbatim / `if .Query.Type` / `if .Keywords`) and forwards
     the Newznab search type end-to-end. The `if .Keywords` motif still
     emits `search` for keyword-less searches: intentional Prowlarr parity.
  2. `unixdate` filter (epoch seconds → RFC3339 UTC, what `fieldsToRelease`
     parses) registered in the exported `filters.Registry`.
  Both ship as **v0.6.0** (public API extended) after the already-approved
  branch migration: FF-merge `feat/modernize-oct2026` (Go 1.27/deps + CI
  rewrite) with its uncommitted WIP (`.golangci.yml` v2, Makefile coverage
  targets, `.editorconfig`) committed first, then worktree and branch
  deleted. **Push + tag only after explicit user confirmation (T4)**; no
  `replace` directive, ever (`go install @latest` would break).
- **gowlarr adapter** forwards `search.Query.SearchType` as
  `cardigannengine.Query.Type` (CLI default `search`, server passes `t`).
- **Native Newznab configurations** (no definition file, documented in
  README + this spec): `baseurl=https://feed.animetosho.net/api/newznab`
  with any placeholder API key (the server ignores it — verified) and
  `baseurl=https://amenzb.moe/api` with a real API key (registration not
  yet obtained — live verification deferred, caps evidence recorded: `GET
  /api?t=caps` answers a full Newznab caps document with `search`/`tv-search`
  and subcat `5070`, while `t=search` without a key returns `<error
  code="100" description="Missing parameter (apikey)"/>`).
- **Newznab client hardening (discovered by the live smokes, fixed within
  the feature)**: (1) cloned shared transports set `ForceAttemptHTTP2`
  unconditionally — `Clone()` runs the source's HTTP/2 auto-setup first, so
  the copy inherits the source's post-setup `TLSClientConfig` (ALPN `h2`)
  but *not* its `TLSNextProto` map; without the force the client advertises
  h2 in ALPN while reading the response as HTTP/1.1 (`malformed HTTP
  response` against any h2-speaking feed). Pinned by
  `TestNew_TransportsForceHTTP2`. (2) Download links prefer the item
  **`<enclosure>`** over `<link>` (per the Newznab spec the enclosure is
  the download asset; the anime mirror's `<link>` is its view page —
  previously fetched and saved as an HTML `.nzb`). Fallback: enclosure →
  link → GUID; pinned by `TestClient_Search_PrefersEnclosure`.
- **Tests are anonymized**: unit/integration tests must not contain any of
  the real site names, hostnames, or release titles — synthetic ids
  (`localjson`), `httptest` URLs, and fictional filenames only. The real
  names live solely in the production yml, README, and this document.
- Verification order: cardigann-go local suite → gowlarr suite → isolated
  live smokes (`XDG_CONFIG_HOME` + `XDG_CACHE_HOME`, cwd = main checkout so
  `definitions-local/` resolves) → independent review → finalize.

## [S3] Out of Scope

- Creating an account on the NZB site requiring an API key (its live
  `indexer test` is deferred until then; no fake config entry).
- `<error>`-element sniffing in Cardigann search responses (a wrong key
  still yields an empty result instead of an error on XML APIs).
- Millisecond `unixdate`, `allowrawsearch`, and the corpus's other absent
  template variables (Artist/Album/Title/Author — empty renders are
  harmless).
- Any `replace`/pseudo-version fallback if the upstream push is declined.
- Body sniffing of downloaded artifacts (an HTML page saved as `.nzb`):
  the resolver has no content guard — pre-existing, owned by the
  search-results-quality surface; the enclosure fix removes the concrete
  trigger found here.
- The same `Clone()` HTTP/2 footgun in cardigann-go's
  `httpclient.buildTransport` proxy branches (the no-proxy path returns the
  shared transport as-is and is unaffected, which is why the local
  definition smokes pass): latent, needs its own upstream tag cycle —
  follow-up.

## Tasks

- [x] T1: cardigann-go branch migration with WIP rescue — acceptance: WIP
      (`.golangci.yml`, Makefile, `.editorconfig`) committed on the branch
      or explicitly abandoned by the user; master contains Go 1.27 + new
      `ci.yml`; worktree and branch deleted without `--force`;
      `make fmt-check && make vet && make test-race` green (covers: S2)
- [x] T2: `engine.Query.Type` + `"search"` default + `engine/querytype_test.go`
      (3 template motifs, default, season-only gate) — acceptance: 4
      scenarios green, `rctx.Query["Type"]` never empty, `defstest` drift
      checked (covers: S2; depends: T1)
- [x] T3: `unixdate` filter + tests — acceptance: `1792340016` →
      `2026-10-18T16:13:36Z` exactly, rejects green, integration date
      non-zero (covers: S2; depends: T1)
- [x] T4: STOP — confirm push + tag `v0.6.0` with the user, then push and
      verify the GitHub Actions run is green via API — acceptance:
      confirmation obtained before any push; tag on origin; CI
      `conclusion=success`; refusal/report → halt, no workaround
      (covers: S2; depends: T1, T2, T3)
- [x] T5: gowlarr `go get cardigann-go@v0.6.0` + tidy — acceptance:
      `go.mod` pins v0.6.0, no `replace`, `go build ./...` green
      (covers: S2; depends: T4)
- [x] T6: adapter forwards `SearchType` + anonymized fixture test with
      `t: "{{ .Query.Type }}"` — acceptance: `tvsearch` and default
      `search` assertions green (covers: S2; depends: T5)
- [x] T7: `definitions-local/aninzb.yml` (real file, main checkout) +
      anonymized integration test (JSON fixture: full item, item without
      `nzb`, `anidb: null`) — acceptance: query params/no-empty-params,
      title/size/absolute link, `PublishDate == time.Unix(1792340016,0).UTC()`,
      conditional details, observed no-nzb behavior, usenet protocol all
      green; `response: type: json` proven by extracted rows (covers: S2;
      depends: T3, T5)
- [x] T8: README (local-defs ingestion note + the two native Newznab
      examples) + this spec — acceptance: both README edits present, spec
      status `in-progress` (delivered belongs to T12) (covers: S2;
      depends: T7)
- [x] T9: live smoke, local definition — `defs sync` ingests the yml,
      `indexer test` OK, pre-validated tvsearch query returns ≥1 result
      with RFC3339 date — acceptance: command outputs recorded
      (covers: S2; depends: T7)
- [x] T10: live smoke, native Newznab mirror — `indexer test` OK, tvsearch
      ≥1 result — acceptance: outputs recorded (covers: S2; depends: T5)
- [x] T11: documented native Newznab integration for the key-requiring site
      — acceptance: README + spec carry the exact config, caps evidence
      recorded, live test marked deferred (covers: S2)
- [ ] T12: independent review green (spec / correctness / consistency,
      including yml↔constant equality) then finalize — acceptance: review
      verdict recorded, spec `delivered` with Report and commit range
      (covers: S1, S2, S3; depends: T6, T7, T8, T9, T10, T11)
