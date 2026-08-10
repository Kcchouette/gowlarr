---
feature: native-newznab-indexer
status: in-progress
updated: 2026-08-10
branch: feat/native-newznab
commits: d10030d..e128d80
---

# Native Newznab Indexer

## Report

**What was built** — Generic Newznab usenet indexers (base URL + API key)
are now first-class persisted indexers: `gowlarr indexer add newznab` (with
the reserved aliases `nzb` and `usenet`) drives the standard settings prompts
through an in-memory synthetic definition (interactive or
`--setting baseurl=... --setting apikey=...`), persists an
`IndexerConfig` under the documented reserved `DefinitionID "newznab"` (no
schema change — `indexer_configs.definition_id` has no FK), and is served by
the native Newznab client in every path: parallel search (CLI + Torznab
server), `indexer test`, and `download` (proxy/DNS preserved on the NZB
fetch). The native client gained per-indexer/global proxy support
(http/https/socks5) and DNS overrides via `NewWithTransport`, and parses the
Newznab `<error>` XML so an invalid API key surfaces as an authentication
failure. The Torznab server now emits `application/x-nzb` enclosures for
usenet results and serves dynamic caps aggregated from the enabled indexers
(native = all five modes; Cardigann defs contribute `caps.modes` with
`audio-search → music-search` normalization).

**Verification** — `make vet` PASS; `make lint` PASS (0 issues, after a
`golangci-lint cache clean` that removed a stale per-module analysis cache);
`go test -race -count=1 ./...` PASS (16 packages); `make build` PASS (the
CLI main package is now tracked — the `.gitignore` rule `gowlarr` was
anchored to the root as `/gowlarr`, un-ignoring the source directory
`cmd/gowlarr/`; the built root binary stays ignored). E2E smoke with the
built binary: `add newznab --setting` prints the settings summary and adds
with instance ID = base-URL hostname; `add usenet` alias hits the
duplicate-ID guard suggesting `--id`; an invalid base URL is rejected;
`indexer test` against the real `api.nzbgeek.info` with a fake key returns
`authentication failed: newznab indexer api.nzbgeek.info: Invalid API Key
(code 101)` (validates the `<error>` parsing live); `search --indexer <id>`
wires the provider (auth error surfaced as a provider warning).

**Journey log** — The cardigann-go engine cannot speak the Newznab HTTP API
(its `torznab` package only emits XML), so a native provider branch is
unavoidable and the sentinel DefinitionID must short-circuit before every
`GetDefinitionYAMLFallback`. Newznab reports a bad API key as HTTP 200 +
`<error>` XML (not a 4xx) — without parsing it, `indexer test` misclassified
auth failures as parsing errors. The `.gitignore` rule `gowlarr` also
swallowed `cmd/gowlarr/` (untracked CLI main package, breaking `make build`
in fresh checkouts) — fixed by anchoring the rule to the root (`/gowlarr`). golangci-lint v2 can serve stale
analysis from its module cache — a clean cache was required for a
trustworthy 0-issue result. Independent reviews (two at plan time, one at
implementation time) caught the error-string case-sensitivity bug, the
missing `download` sentinel branch, and the socks5 DNS-composition comment.

## [S1] Problem

A generic Newznab usenet indexer (base URL + API key — nzbgeek, nzb.su,
NZBHydra…) cannot be added as a persistent indexer in gowlarr. The native
Newznab client (`internal/newznab`) exists and implements `search.Provider`,
but it is only reachable ad-hoc through `gowlarr search
--newznab-url/--newznab-apikey` flags (nothing persisted). All persisted
indexers are instances of Cardigann definitions; the cardigann-go engine
does not speak the Newznab HTTP API (its `torznab` package only emits XML),
so a generic Newznab indexer cannot be expressed as a scraping definition.

Consequences: no `indexer add`/`test`/persisted search for native Newznab
indexers, and the Torznab server serves usenet results with the wrong
enclosure type (`application/x-bittorrent` hardcoded) and hardcoded caps.

## [S2] Design

Settled decisions (user-approved plan
`.mimocode/plans/1786376023253-kind-otter.md`, validated by two independent
reviews):

- **Persistence — sentinel DefinitionID, zero migration.** New documented
  constant `store.NewznabDefinitionID = "newznab"` in
  `internal/store/indexers.go`. `indexer_configs.definition_id` has no FK to
  `indexer_definitions`, so no schema change. Configs with this ID are served
  by the native client; every code path that would load a definition for
  this ID branches BEFORE the `GetDefinitionYAMLFallback` lookup.
  `Settings = {baseurl, apikey}`; `Protocol = "usenet"`.
- **CLI — `gowlarr indexer add newznab|nzb|usenet`.** Exact case-insensitive
  query interception in `newIndexerAddCmd` before definition resolution. A
  synthetic in-memory `definition.IndexerDefinition` (settings: `baseurl`
  text, `apikey` password, one `info` help line) drives the existing
  `requiredSettings`/`runPrompts`/`printSettingsSummary` machinery unchanged
  (interactive AND `--setting key=value`). Default instance ID = hostname of
  base URL (overridable `--id`); helpers `isNewznabAlias`/`newznabDefaultID`.
- **Service.** `IndexerService.AddNewznab(instanceID, baseURL, apiKey,
  proxyURL) error`: validates base URL (http/https + host), rejects a
  duplicate instance ID (SaveIndexerConfig upserts silently on `id`),
  persists the native config. `IndexerService.Test` branches on the sentinel
  and classifies errors on the **lowercased** message: HTTP 401/403 or
  "invalid api" → "authentication failed", else "network/parsing error".
- **Provider building.** Shared helper
  `newznabProviderFromConfig(cfg, appCfg)` (missing baseurl/apikey →
  error, caller warns+skips). Sentinel branches at the top of
  `BuildConfiguredProviders`, `BuildIndexerProvider`,
  `buildIndexerProviderFromRaw`, before any definition lookup.
- **Transport.** New constructor
  `newznab.NewWithTransport(id, name, baseURL, apiKey, proxyURL, overrides)
  (*Client, error)`; `New`/`NewWithOverrides` delegate (proxy `""` can never
  fail). Per-client clone of the shared transport: http/https proxy via
  `http.ProxyURL`, socks5 via `golang.org/x/net/proxy` (promoted to a direct
  dependency), DNS overrides composed last + `ForceAttemptHTTP2 = true` when
  a DialContext is set. No FlareSolverr, no browser-UA (anti-bot measures
  for scraped HTML, not XML APIs).
- **API endpoint (amendment).** The client computes the API endpoint once
  (`Client.apiURL` via `apiEndpoint`): a bare origin gets `/api` appended
  (standard Newznab, e.g. SpotWeb); a base URL carrying a custom path is
  used verbatim (e.g. Hydra's `api/v1/newznab`). Applied to both `Search`
  and `Caps`, backward compatible (existing configs are bare origins).
- **Newznab `<error>` XML.** A bad API key returns HTTP 200 +
  `<error code="100" description="Invalid API Key"/>`; `Search` parses the
  root element and returns `newznab indexer %s: %s (code %s)` instead of a
  misleading RSS parse failure, so `indexer test` reports authentication
  failures correctly.
- **Server.** `WriteTorznabResponse` sets the enclosure type per protocol
  (`application/x-nzb` for usenet, else `application/x-bittorrent`).
  `handleCaps` is dynamic: aggregated from enabled indexers — the sentinel
  declares all five modes; Cardigann defs contribute `def.Caps.Modes` with
  `audio-search → music-search` normalization; a mode is `available="yes"`
  if any enabled indexer supports it. `store == nil` (tests) keeps the
  all-yes fallback; an empty store yields all-`no`. Categories unchanged.
- **Download.** `DownloadService.buildClient` branches on the sentinel
  before the definition lookup and returns the native client's HTTPClient
  (proxy/DNS preserved); Newznab enclosure URLs embed the API key, so no
  auth headers are needed.
- **Data flow.** `add newznab` → `AddNewznab` → `SaveIndexerConfig`
  (SQLite). Search/server/test → sentinel branch → `newznabProviderFromConfig`
  → `NewWithTransport` → `<apiURL>?t=...&apikey=...` (`/api` appended to a
  bare origin, custom path kept verbatim) → `ReleaseInfo{Protocol: usenet}`.
  Server serves it with `application/x-nzb` enclosures and dynamic caps.

## [S3] Out of Scope

- REST CRUD for indexers (CLI-only, consistent with the existing surface).
- FlareSolverr / browser-UA for the native client.
- Live per-indexer caps queries (static aggregation from definitions:
  offline, cheap, deterministic).
- Settings encryption (existing `crypt` infra, unwired — unchanged).
- Augmenting an NZB enclosure URL that lacks the API key at download time
  (standard Newznab indexers embed the key in the URL).
- `defs` commands (the sentinel lives only in `indexer_configs`).

## Tasks

- [x] T1: spec + workspace (worktree `feat/native-newznab`) — acceptance:
      this document committed as `status: in-progress` on the feature branch
- [x] T2: `store.NewznabDefinitionID` constant — acceptance: `go build ./...`
      green (covers: S2)
- [x] T3: `newznab.NewWithTransport` + `<error>` XML parsing in `Search` +
      delegating `New`/`NewWithOverrides`; go.mod promotes `x/net` —
      acceptance: new client tests green (proxy/socks5/overrides/error XML)
      (covers: S2)
- [x] T4: `IndexerService.AddNewznab` + sentinel branch in `Test` —
      acceptance: service tests green (roundtrip, duplicate ID, invalid URL,
      401/bad-key/OK) (covers: S2; depends: T2, T3)
- [x] T5: `newznabProviderFromConfig` + sentinel branches in the three
      provider builders — acceptance: provider tests green (covers: S2;
      depends: T4)
- [x] T6: CLI interception `isNewznabAlias`/`newznabDefaultID` + synthetic
      definition + `AddNewznab` call — acceptance: helper unit tests green;
      manual `add newznab` interactive and `--setting` paths work
      (covers: S2; depends: T4)
- [x] T7: server enclosure-per-protocol + dynamic caps — acceptance: server
      tests green (enclosure, dynamic caps with real store, nil-store
      fallback) (covers: S2)
- [x] T8: `DownloadService.buildClient` sentinel branch — acceptance:
      download test green (covers: S2; depends: T5)
- [x] T9: README section for the persisted form — acceptance: `add newznab`
      documented next to the ad-hoc usage (covers: S2)
- [x] T10: verification + independent review + finalization — acceptance:
      `make test`/`vet`/`lint`/`build` green, review findings addressed,
      spec `delivered` with `Report` and commits range (covers: S1-S3;
      depends: T2-T9)
- [ ] T11: (amendment) API endpoint custom — `Client.apiURL`/`apiEndpoint`
      (Search + Caps), tests `TestClient_*_APIEndpoint`, CLI info text +
      README custom-path note, spec amended — acceptance: `make
      vet/lint/test/build` green, tests verts, `add newznab --setting
      baseurl=<custom-path>` fonctionne et `indexer test` tape le bon chemin
      (covers: S2)
