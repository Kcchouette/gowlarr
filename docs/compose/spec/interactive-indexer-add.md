---
feature: interactive-indexer-add
status: delivered
updated: 2026-08-02
branch: interactive-add
commits: afe81b0..c9a74b2
---

# Interactive `gowlarr indexer add`

## Report

**What was built** — `gowlarr indexer add <query>` is now interactive when the definition needs it: after resolution, the CLI loads the definition with the exact version the service will persist (`best.Version`), computes the required settings (`text`/`password` without a Default, in definition order), and — when stdin is a terminal — prints the definition's `info` help texts once (e.g. abnormal's "Find your API key on your Parameters page…") then prompts each missing setting by label, with `password` inputs masked via `charmbracelet/x/term` (already an indirect dependency, promoted to direct with zero new downloads). A settings summary (password as `*****`, unknown `--setting` keys listed after, sorted) is shown before persisting. Prompts and summary go to **stderr**, keeping stdout clean for the "Resolved"/"Indexer added" lines. `--setting key=value` remains the prioritaire non-interactive path; when required settings are missing and stdin is not a terminal (pipe, or `CI` set), `add` fails with an error listing the keys and the `--setting` hint, persisting nothing. SIGINT during a masked read is intercepted so x/term's echo-restoring defer runs (clean terminal, exit 1). `gowlarr indexer add abnormal-api` now asks `APIKey:` after showing the Parameters help; `gowlarr indexer add japanfan` asks `Username:` then `Password:`.

**Verification** — `go test -race -count=1 ./...` green; `go vet ./...` clean; `golangci-lint run ./...` 0 issues; `go mod tidy` stable (`git diff --exit-code go.mod go.sum` empty — only `charmbracelet/lipgloss` + `x/term` promoted to direct, no version change). New tests: `TestRequiredSettings` (10 cases), `TestStdinPrompterPlain` + `TestStdinPrompterPlainEOF`, `TestRunPrompts` (6 cases), `TestPrintSettingsSummary` (4 cases), `TestIndexerAddCmdNonTTYMissingSettings` (pipe stdin, error with keys+help+hint, nothing persisted), `TestIndexerAddCmdInteractiveFillsSettings` (fake prompter, merged settings persisted). Fresh reviewer: APPROVE (only minor notes: paste-ahead bufio over-read accepted/TTY-only, unknown-key echo on stderr spec-aligned, untracked docs/ fixed by this commit).

**Journey log** —
- `golang.org/x/term` MVS-selects v0.45.0 whose zip is not cached, while `charmbracelet/x/term v0.2.1` (the fork, same API, `uintptr` fd) is already an indirect dependency with the zip cached — zero-fetch win.
- x/term's `ReadPassword` restores terminal echo only via a `defer`; with the default SIGINT handler the process dies before it runs, leaving echo disabled. Intercepting SIGINT (`signal.Notify`) makes the read return EINTR, the defer runs, the error propagates — exit 1 with a clean terminal.
- A masked prompt must print its label before the masked read, or the user types blind; the label was hoisted out of the plain-only branch after review.
- Prompts and the settings summary belong on stderr: on stdout they'd be swallowed by redirection (`add > log`) leaving the user typing into an empty terminal, and they'd pollute scripted stdout.
- `best.Version` from `ResolveDefinition` is the highest-priority *parseable* version — strictly more robust than the old `GetDefinitionYAMLFallback` (which could pick an unparseable row), same priority order.

## [S1] Problem

`gowlarr indexer add <query>` accepts credentials only via `--setting key=value` flags. A definition like japanfan (username/password) or abnormal-api (API key, with a "find your key on your Parameters page" hint) added without flags silently persists empty settings and only fails later at `indexer test`/first search. There is no guided path and no display of the definition's own help texts (`settings[].type: info`).

## [S2] Design

Settled contracts (validated by two independent reviews before implementation):

1. **Auto + TTY guard**: prompts fire only when (a) required settings are missing from the `--setting` map AND (b) stdin is a terminal (`charmbracelet/x/term.IsTerminal`). `CI` env forces non-interactive. Non-TTY + missing → error on **stderr** listing the missing keys, the definition's `info` help texts, and a `--setting key=value` hint. `--setting` stays prioritaire and non-interactive.
2. **Required-only + info help**: prompt only `text`/`password` settings **without a Default**, in definition order. Settings with a Default are never prompted (the engine injects them via configOnce). `type: info` settings are not inputs; their Default text is printed once as help before the inputs. checkbox/select/unknown types are never prompted.
3. **Masked password** via `github.com/charmbracelet/x/term v0.2.1` (already an indirect dependency, cached — zero network fetch): `term.ReadPassword(os.Stdin.Fd())`, label printed before the masked read, newline after.
4. **Prompts + summary → stderr** (`cmd.ErrOrStderr()`): stdout keeps only the "Resolved"/"Indexer added" lines, so stdout redirection never swallows prompts and scripted adds see no new stdout block. The pre-save summary shows label + value, passwords as `*****`, unknown keys (--setting typos) listed after, sorted.
5. **SIGINT during masked read**: `signal.Notify` intercepts os.Interrupt around ReadPassword so x/term's defer restores echo before the (non-fatal) error propagates — exit 1 with a clean terminal.
6. **Version coherence**: the CLI loads the definition with `store.GetDefinitionYAML(defID, best.Version)` and passes `best.Version` to `svc.Add` — same row the service persists, no divergence possible.
7. **CI guard**: `os.Getenv("CI") != ""` is treated as non-TTY (avoids hangs in CI-with-pty / ssh -t / tmux sessions). Documented.

Edge cases: empty input accepted as `""` (no retry); Ctrl-D on a plain prompt with no line → "input closed (EOF)" error, partial line accepted; `--setting key=` counts as provided (empty value); unknown `--setting` keys persisted as today and surfaced in the summary's extra list.

## [S3] Out of Scope

- `gowlarr indexer set` (update settings of an existing instance) — remove + re-add remains the workaround.
- Editable default values at add time (rejected: requires-only; a custom-domain `apiurl` can still be overridden via `--setting`).
- Automated PTY testing of the masked read path (creack/pty not worth a new dependency) — documented limitation; the label print shares the plain-path code.
- Prompts on stdout (deliberately stderr).
- `select`-without-default prompting (documented, rare in corpus).

## Tasks

- [x] T1: `requiredSettings` + `TestRequiredSettings` — acceptance: `go test -race ./internal/cli/ -run TestRequiredSettings` green (covers: S2.2)
- [x] T2: prompt machinery (seams `stdinIsTerminal`/`newStdinPrompter`, `prompter`, `stdinPrompter` with hoisted label, masked read + SIGINT, EOF handling) — acceptance: build + `TestStdinPrompterPlain` green (covers: S2.1, S2.3, S2.5, S2.7)
- [x] T3: `runPrompts` + `printSettingsSummary` — acceptance: `TestRunPrompts`/`TestPrintSettingsSummary` green (covers: S2.2, S2.4)
- [x] T4: CLI wiring (stderr, non-TTY error + info help, best.Version) + `TestIndexerAddCmdNonTTYMissingSettings` + `TestIndexerAddCmdInteractiveFillsSettings` — acceptance: both CLI tests green; full matrix green (covers: S2.1, S2.4, S2.6)
- [x] T5: spec finalize + `go mod tidy` (promote charmbracelet/x/term to direct) + full verification — acceptance: matrix green; `git diff --exit-code go.mod go.sum` clean (covers: S1)
