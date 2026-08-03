---
feature: search-results-quality
status: in-progress
updated: 2026-08-03
branch: search-results-quality
commits: pending
---

# Search Results Quality

## Report

<!-- filled at delivery -->

## [S1] Problem

Two user-visible defects on `gowlarr search` for the The Lost Space indexer
(a MyBB usenet forum, definition `definitions-local/thelostspace.yml`):

1. **No dates** — the AGE column printed `-` for every row: the definition
   extracted no `date` field, so `ReleaseInfo.PublishDate` stayed zero.
2. **Non-release threads in results** — a chit-chat thread such as
   "Voili voilou" (community forum) matched search keywords and appeared as a
   result; nothing filtered it.

Additionally the definition's `download` field selected an attachment link
that never exists on the search results page, so every `DownloadLink` was
empty (rows were undownloadable even via `--links`).

## [S2] Design

Settled decisions (user-selected, after live investigation of the real site
HTML with the stored session):

- **Dates: extract only** — display stays relative (AGE). The MyBB advanced
  search (`showresults=threads`) renders one result per thread; the last-post
  date lives in the 7th `<td>` as
  `td:nth-child(7) span.smalltext` with text `"11.05.2026, 16:20 Dernier
  message: <author>"`. Extract the date part with a `regexp` capture
  (`[0-9]{2}\.[0-9]{2}\.[0-9]{4}, [0-9]{2}:[0-9]{2}`) then `dateparse` with
  .NET args `"dd.MM.yyyy, HH:mm"`. Field is `optional: true` so a structure
  drift degrades to `-`, never to a lost row.
- **Junk: category-based filtering at the source** (chosen over inspecting
  each thread page for an NZB link): restrict the search to release forums.
  MyBB casts a scalar `forums` param to `int`, so a comma list silently
  reduces to the first fid (verified live: `forums=4,5,8,25,70,75` returned
  only fid 31); the array form `forums[]=` is required (verified live:
  subforum fid 48 under 25 is included). Release top-level fids (subforums
  included): 4 FILMS, 5 SÉRIES, 8 INTERNATIONALE, 25 JAPANIMATION,
  70 LOGICIELS, 75 GAMES. Community/info fids (1,2,3,10,11,12,13,14,52,64,
  65,72,73,79,80,81) never match. Emitted via the `$raw` input.
- **Generic safety-net filter in the engine**: `internal/search/engine.go`
  drops releases with no usable link (`DownloadLink`, `InfoHash` or
  `StreamURL` all empty) inside the per-provider goroutine, BEFORE the
  per-provider cap, so junk rows never consume the result budget. To survive
  it, thelostspace rows carry the thread URL as `download` field
  (`a[href*="showthread.php?tid="]` attribute href — non-empty for every
  row).
- **On-demand NZB chain** (WIP `7f8530c` semantics preserved — no chain at
  search time): a top-level `download:` block follows the thread page (plain
  GET, no side effect) and extracts `a[href*="attachment.php?aid="]` from it
  with `usebeforeresponse: true`; it runs only via `--links`
  (`ResolveDownloadLink`). The engine returns the raw relative href, so the
  adapter absolutizes the resolved link against `def.Links[0]` (RFC 3986
  `ResolveReference`; equals the provider base URL in production).
  Search-time stored links are NOT absolutized: `gowlarr download <ID>`
  keeps failing fast on the relative URL instead of silently saving HTML as
  `.nzb`.

## [S3] Out of Scope

- Full DATE column in the CLI (AGE stays relative).
- Direct `gowlarr download <ID>` for The Lost Space (requires `--links`
  first; a follow-up could run the chain inside the download path).
- Row-level fid exclusion: cardigann-go `rows.filters` has no exclude-by-value
  filter, so a junk thread posted inside a release forum remains visible.
  The thread-view paperclip indicator (attachment presence per row) is a
  possible future signal but needs an engine change.
- Forum timezone handling (dates parsed as UTC; ≤ 2h skew on AGE).
- Loosening cardigann-go's date parser (RFC3339-only without `dateparse`).
- Pre-existing `--indexer` matching quirk: the flag compares the provider
  definition id (`thelostspace`) with the configured instance id
  (`thelostspace.xyz`); cardigann providers never match. Not touched here.

## Tasks

- [x] T1: engine usable-link filter + test updates (6 artifacts incl. bench)
  — acceptance: `go test -race -count=1 ./internal/search/` green (covers: S2)
- [x] T2: absolutize `ResolveDownloadLink` output in the adapter + unit tests
  — acceptance: `go test -race -count=1 ./internal/cardigannadapter/
  ./internal/service/` green (covers: S2; depends: T1)
- [x] T3: thelostspace.yml (showresults/forums[]/date/download chain) +
  inline-YAML fixtures (search with SkipSearchDownload, chain with
  absolutization) — acceptance: `go test -race -count=1
  ./internal/cardigannadapter/ -run TheLostSpace` green, live search shows
  AGE dates and no community rows (covers: S2; depends: T2)
- [ ] T4: spec finalization + full verification matrix — acceptance: full
  suite/vet/lint green, live checks recorded (covers: S1-S3; depends: T3)
