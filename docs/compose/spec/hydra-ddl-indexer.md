---
feature: hydra-ddl-indexer
status: in-progress
updated: 2026-08-03
branch: hydra-ddl
commits: # filled at delivery
---

# Hydra DDL Indexer

## Report

<!-- filled at delivery -->

## [S1] Problem

Hydracker (hydracker.com), a French tracker, exposes both a Usenet/newznab
API (already covered by `definitions-local/hydra.yml`, `type: usenet`) and a
Direct-Download section (DDL, 1Fichier hosts) browsable per title/season/
episode. The DDL section is not searchable from gowlarr: there is no way to
ask `gowlarr search "<titre>" --indexer <hydra-ddl>` and get the DDL items
(episode or full-season × quality × host) with their size, date and category.

Live investigation of the site's JSON API (`/api/v1/`) established:

- Title search is public: `GET /titles?query={q}` returns the best matching
  title first (`pagination.data[0]`), with `id`, `name`, `type`, `year`.
- The full lien list of a title is one authenticated request:
  `GET /titles/{id}/content/liens?loader=linksdl` returns every DDL lien
  (226 for House of the Dragon) as `{"liens":[{...}],"count":N}` — no
  pagination wrapper. Requires `Authorization: Bearer <token>` (personal
  access token, creatable by any account; works on the free tier).
- The actual file URL is NOT obtainable programmatically: resolving
  `GET /content/liens/{lienId}` returns 403 `premium_required` for a free
  account (premium-only API) and additionally requires a Cloudflare
  Turnstile token. The download page JSON (`.../download`) always has an
  empty `src`.
- Session login is impractical for automation: the web login page requires a
  Cloudflare Turnstile widget; the API login (`POST /auth/login`) requires a
  dynamic CSRF (`X-XSRF-TOKEN` from a cookie) that the cardigann engine
  cannot produce.

Engine constraint: a Cardigann search is ONE HTTP request per `search.path`
(`engine.go` iterates paths without chaining). The hydra DDL flow is two
steps (keywords → title id → liens), so a plain definition cannot express
it.

## [S2] Design

Settled decisions (user-approved plan `1785764357576-quiet-wolf.md`,
validated by two independent reviews):

- **New engine capability `search.before`** (cardigann-go fork, branch
  `feature/search-before`): an optional pre-search request chain. Each step
  is a GET/POST whose named selectors capture values into the template
  context as `{{ .Before.<stepName>.<selectorName> }}`, usable by the search
  path templates. Contract:
  ```yaml
  search:
    before:
      - name: title            # required, [A-Za-z0-9_]+
        path: titles           # template
        method: get            # get | post (default get)
        inputs: {query: "{{ .Keywords }}"}
        response: {type: json} # json | xml | html (default html)
        selectors:
          id: "$.pagination.data.0.id"
          name: "$.pagination.data.0.name"
    headers: {Authorization: "Bearer {{ .Config.apikey }}"}  # applied to before steps too
    paths:
      - path: "titles/{{ .Before.title.id }}/content/liens"
        response: {type: json}
  ```
  Implementation notes (fork):
  - `definition`: `SearchBefore{Name,Path,Method,Inputs,Response,Selectors}`,
    `SearchBeforeList` with dual map/list `UnmarshalYAML` (same pattern as
    `DownloadBeforeList`); `Search.Before` mirrored in
    `Search.UnmarshalYAML` (explicit `rawSearch` copy — omitting it silently
    drops the field).
  - `template.Context.Before` is `map[string]map[string]string` keyed by step
    name (a flat map cannot render `{{ .Before.title.id }}`).
  - `engine`: extract the header-rendering loop into a shared helper; run the
    before steps after the empty-search gate, before the paths loop; reuse
    `filterInputs` semantics of `searchPath`; JSON extraction at document
    level (gjson, `$.` trimmed) via a new helper mirroring
    `extractFromResponse`.
  - Failure semantics: an HTTP/auth failure on a before step aborts the
    search with an error (`searchError`, matching the existing
    `searchError`/`loginError` precedent — a bad PAT must be visible, not a
    silent "0 results"); a successful request whose required selector
    matches nothing is a legitimate "no results" (empty return, no search
    request fired).
  - All behaviors covered by unit tests (parsing single+list, JSON/HTML/POST
    steps, chained steps, header templating, empty-search gate, soft-fail,
    concurrency, regression).
- **Definition `definitions-local/hydraddl.yml`** (`id: hydraddl`,
  `type: ddl`):
  - `settings.apikey` = personal access token (Bearer), same shape as the
    existing `hydra.yml` apikey setting.
  - `search.before` step `title`: `titles` + `inputs.query` = keywords
    (public, no auth needed); captures `id`, `name`, `type`.
  - `search.paths`: `titles/{{ .Before.title.id }}/content/liens` with
    `loader: linksdl` and optional `season: {{ .Query.Season }}` (filtered
    when empty); rows selector `$.liens`.
  - Fields: flat helper fields `_season`, `_episode`, `_full_saison`,
    `_quality` (`qual.qual`), `_hostname` (`host.name`) — `.Result` is a flat
    map of declared fields, nested access is impossible and `"0"` is truthy,
    so the title template uses `{{ if eq .Result._full_saison "1" }}` and
    flat references; `title` = `"<title.name> S<s>E<e> - <quality>
    [<host>]"` (or `"S<s> (saison complete)"`); `size` = raw bytes
    (`taille`); `date` = `created_at` + `dateparse`; `category` = `case`
    mapping of the captured title `type` with fallback `*: 8000`;
    `details`/`download` = **absolute** public page URLs
    (`https://hydracker.com/titles/...`), display-only.
  - No `login` block (auth by header) and no `download:` chain (real link is
    premium+Turnstile-gated).
- **gowlarr integration**: no Go code change expected — the extension lives
  in the fork; the definition emits absolute URLs (search results are not
  absolutized by the adapter; `ResolveDownloadLink` already absolutizes for
  `--links`). Test: adapter fixture (inline YAML + `httptest.Server`
  two-step API), matching `internal/cardigannadapter/fixture_test.go`
  conventions.

## [S3] Out of Scope

- Real 1Fichier link resolution: premium-only API (`premium_required`) plus
  Cloudflare Turnstile → rows link to the public download page
  (`display-only`, consistent with gowlarr's DDL model).
- Multi-title search: only the first title match is used (engine constraint:
  one search request).
- Episode-level filtering for `tv-search`: the API's lien datatable supports
  `season` only; all episodes of the season are returned.
- Automated session login (web login has Turnstile; API login needs dynamic
  CSRF unsupported by the engine).
- Pre-existing `--indexer` quirk: the flag matches the provider definition id
  (`hydraddl`), while `indexer add` stores the instance by domain
  (`hydracker.com`); the working invocation is `--indexer hydraddl`. Not
  touched here.

## Tasks

- [ ] T1: fork `search.before` — definition structs + dual-form parsing +
  `Context.Before` + engine `runSearchBefore` + tests (single/list, JSON/
  HTML/POST, chained steps, header templating, empty-search gate, soft-fail
  semantics, name validation, regression) — acceptance: `go test ./...` and
  `go vet ./...` green in the fork (covers: S2)
- [ ] T2: `definitions-local/hydraddl.yml` per the S2 contract — acceptance:
  `gowlarr defs sync && gowlarr defs list` shows it and `defs show hydraddl`
  resolves; the only `Validate()` warning is the expected
  "ddl definition without download field" (by design: no resolution chain,
  rows link to the public download page). The file is gitignored
  (`definitions-local/`), so the shipped artifact is the inline fixture in
  T3 (covers: S2; depends: T1)
- [ ] T3: gowlarr adapter fixture test (two-step API server, Bearer header,
  rows/fields, zero-results on no match, error surfaced on 401) —
  acceptance: `go test ./internal/cardigannadapter/` green (covers: S2;
  depends: T1)
- [ ] T4: live verification against hydracker.com with the provided PAT —
  acceptance: `gowlarr search "house of the dragon" --indexer hydraddl`
  returns rows (up to the pre-existing per-provider cap of 10,
  `defaultMaxResultsPerProvider`) with correct sizes/dates/categories (5000)
  and absolute download-page links, matching the raw
  `api/v1/titles/179751/content/liens?loader=linksdl` response (226 liens);
  `--season 2` filters to season 2 (covers: S2; depends: T2, T3)
- [ ] T5: spec finalization (delivered) + full verification matrix recorded —
  acceptance: full suites green, live checks recorded (covers: S1-S3;
  depends: T4)
