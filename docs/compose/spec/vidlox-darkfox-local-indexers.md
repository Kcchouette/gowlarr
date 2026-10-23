---
feature: vidlox-darkfox-local-indexers
status: delivered
updated: 2026-10-24
branch: feat/vidlox-darkfox-local-indexers
commits: d4526db..5447d5a
---

# Vidlox / Darkfox local indexers (torrent magnet)

## Report

**What was built** — Two gitignored local Cardigann definitions (`definitions-local/vidlox.yml`, `darkfox.yml`) for French torrent clone sites: `GET /recherche/{{ .Keywords }}`, table rows, title with `remove: span.WinOption1`, all site categories, magnet only on the `/detail/{id}` fiche via `download.before` + `a[href^="magnet:"]`. **cardigann-go** `ResolveDownloadLink` is now the strict download-time counterpart of `SkipSearchDownload` (magnet short-circuit, InfoHash→bare magnet synth, hard error when a required chain extracts nothing; search-time soft-fail unchanged). **gowlarr** `DownloadService.Resolve` runs that chain before `download.Resolver` for torrent/usenet, builds the fetch client after the chain, and skips chain for ddl/streaming and already-magnet links.

**Verification** — cardigann-go: `go test ./...` all packages ok (new ResolveDownloadLink suite: magnet fiche, no-magnet error, already-magnet no-op, InfoHash synth, no-block no-op, row-only no strict). gowlarr (GOWORK → local cardigann): `go test ./...` all packages ok; `go build` OK; `golangci-lint run` 0 issues. Committed tests: `TestLocalMagnetDefinition_Search`, `TestLocalMagnetDefinition_ResolveLinks`, `TestDownloadService_Resolve_MagnetFromFiche`, `_AlreadyMagnet_SkipsChain`, `_ChainFailure_Errors`, `_resolveChain_NoDownloadBlock_KeepsStoredLink`, `_DDL_SkipsChain`. Site names absent from `*_test.go`. Production YAMLs `definition.Parse` OK.

**Journey log**

1. Prowlarr Pattern 1 (torrent9.yml) = fiche URL + on-demand magnet scrape; approach A (poster `img src` == btih) rejected as CDN-invariant.
2. Soft-fail search rows vs strict download-time is the correct SRP seam; only `ResolveDownloadLink` hardened.
3. `usebeforeresponse` infohash double-Find needs nested DOM (same quirk as kinozal fixture).
4. go.work must live outside both repos and point at the **worktree**, not the main gowlarr checkout.
5. `gowlarr download` after `--links` must short-circuit on stored `magnet:` — chain would re-fetch and hard-fail.

## [S1] Problem

Two French torrent clone sites (same engine, shared catalog) are not indexed. Magnets live only on the `/detail/{id}` fiche. `gowlarr download` never runs the Cardigann `download.before` chain, so a torrent-type release whose stored link is the fiche HTML hits `looksLikeHTML` with a confusing error — magnets never resolve. The same gap blocks usenet attachment chains (e.g. thelostspace).

## [S2] Design

**Approach B** (Prowlarr `torrent9.yml` pattern): search stores the fiche URL; an on-demand `download.selectors` chain extracts `a[href^="magnet:"]` at download / `--links` time.

### cardigann-go v0.8.0 (co-dev branch `feat/download-chain-strict`)

`ResolveDownloadLink` becomes the strict download-time counterpart of `SkipSearchDownload`:

- no-op when `def.Download == nil` or empty stored link;
- no-op when stored link already has `magnet:` prefix (results persisted after `--links`);
- run chain with `startURL` = stored link;
- if `res.URL` set → use it; else if `res.InfoHash` set → synthesize `magnet:?xt=urn:btih:<hash>`;
- else if definition has `before` steps or `usebeforeresponse` selectors → hard error `download chain did not resolve a link from %q`;
- else keep stored link (row-only selectors, download-time no-op).

Search-time soft-fail in `extract.go` is unchanged. `--links` bulk (`resolveLinks`) still warns and keeps the raw link on error.

Tag `v0.8.0` (behavior change: silent-keep → hard error on explicit download-time resolve) must be pushed before gowlarr `go.mod` pin bump.

### gowlarr `DownloadService.Resolve`

1. Load result; if protocol ddl/streaming → `resolver.Resolve` only (refuses display-only).
2. If stored `DownloadLink` has `magnet:` prefix → `resolver.Resolve` only (IsMagnet short-circuit, zero chain HTTP).
3. Else for torrent/usenet: `buildIndexerProvider` + type-assert `linkResolver` → `ResolveDownloadLink`; on error, fail loud (no HTML-as-torrent fallthrough). Provider-build failure keeps the documented warn+fallback exception.
4. Build resolver client **after** the chain; `resolver.Resolve` (magnet short-circuit already inside).

`search --links` unchanged (`SearchService.resolveLinks`).

### Local definitions

Two gitignored YAMLs (`definitions-local/vidlox.yml`, `darkfox.yml`), identical except `id` / `name` / `links[0]`:

- `type: public` → protocol torrent;
- search path `GET /recherche/{{ .Keywords }}`;
- rows `table.table-hover tbody tr:has(td.liste-accueil-nom)`;
- title `a.ShowOption1` + `remove: span.WinOption1` + htmldecode/strip;
- download/details = fiche href;
- category CSS `case` on category links (films/séries/musiques/ebooks/logiciels/jeux-pc/jeux-consoles → Newznab ids);
- seeders/peers optional; size/date omitted (French relative / human size, no filters);
- `download.before` + `a[href^="magnet:"]` `usebeforeresponse`.

### Tests

Anonymized inline fixtures (`dle_clone_local_test.go` pattern): search shape + resolve-links magnet. Service-level tests for short-circuit, strict error, no-op without download block, ddl skip. cardigann-go engine tests for strict/magnet-synth.

## [S3] Out of Scope

FlareSolverr/captcha; category browse without keywords (unless `/recherche/` empty is broken); tracker injection beyond page magnet; commit of `definitions-local/` YAMLs; exposing provider HTTP client (change 2); rewriting `download.Resolver` itself for intermediate pages.

## Tasks

- [x] T1: Worktrees + go.work + spec — acceptance: worktree `feat/vidlox-darkfox-local-indexers`, cardigann branch `feat/download-chain-strict`, go.work outside repos, this spec exists (covers: S2)
- [x] T2: cardigann-go strict + magnet short-circuit + InfoHash synth — acceptance: `go test ./engine/` green; tests i–vii (covers: S2)
- [x] T3: DownloadService chain + short-circuit — acceptance: service tests a–e green (covers: S2; depends: T2)
- [x] T4: YAML vidlox/darkfox + anonymized Cardigann tests — acceptance: Parse OK; Search + ResolveLinks tests green; site names absent from tests (covers: S2; depends: T2)
- [x] T5: Batch verify + pin v0.8.0 + finalize — acceptance: both modules test/build/lint green; **local tag v0.8.0 @ 862e962**; **pin bump deferred until tag is pushed** (CI cannot fetch local-only tag); spec delivered (covers: S2; depends: T2, T3, T4)
