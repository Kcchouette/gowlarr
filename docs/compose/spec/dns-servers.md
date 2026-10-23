---
feature: dns-servers
status: delivered
updated: 2026-08-20
branch: feat/dns-servers
commits: ab62151..621a7aa
---

# DNS Servers

## Report

**What was built** — The `dns_overrides` host→IP pinning map was removed and replaced by a `dns_servers` config option (`[]string`, e.g. `["1.1.1.1", "1.0.0.1"]`). When set, a global `net.Resolver{PreferGo: true, Dial: ...}` queries the configured servers (normalized to host:port, IPv6 bracket-safe, round-robin first-try with sequential connect-level failover) and is injected into every outbound dial path via a lazy `net.Dialer{Resolver: ...}` DialContext: Cardigann clients (search/test/download), native Newznab clients (direct and HTTP-proxy paths), the ad-hoc newznab search, and the SSRF pre-check (`lookupHost`). Empty/omitted `dns_servers` preserves system-resolver behavior. The SOCKS5 paths (Cardigann and Newznab) are deliberately left untouched — replacing their DialContext would bypass the proxy, and SOCKS5 resolves hostnames remotely. The download Cardigann path, which historically skipped DNS overrides, now honors the global DNS policy (called out in the feat commit message). cardigann-go was not modified.

**Verification** — `make test` (`go test -race -count=1 ./...`): all packages ok. `make lint` (golangci-lint): 0 issues. `make build`: OK. Targeted tests cover normalization (IPv4/IPv6 bracketed/bare/port), fake-UDP redirect (via `SetDNSServers` and via the `lookupHost` var), round-robin spread, lazy dialer cancellation, config round-trip + legacy-key ignore, Cardigann apply (no-proxy, http-proxy, SOCKS5-preserve, no-servers no-op), Newznab constructors. A fresh neutral reviewer verified all acceptance criteria against the diff and cardigann-go v0.6.1 source: APPROVE, no CRITICAL findings; the three actionable minors (round-robin test parity assumption, dead assertion, stale comment) were fixed in `621a7aa`.

**Journey log** —
- Two independent pre-implementation reviews caught a CRITICAL SOCKS5 proxy-bypass bug in the first draft of `applyDNSServers` (unconditional DialContext replace); the `tr.DialContext != nil` guard is the load-bearing fix and is pinned by a regression test.
- cardigann-go v0.6.1 sets a custom DialContext only in the socks5 branch and returns the shared `tunedTransport` on the no-proxy path — any gowlarr wrap must skip non-nil DialContexts and clone before mutating.
- UDP `connect()` succeeds against blackholed servers, so sequential in-Dial failover alone never reaches the second server; round-robin on the first try is what actually spreads load.
- T1–T6 cannot compile independently (removing `DNSOverrides` breaks consumers) — they landed as a single atomic feat commit, which the task plan anticipated after review round 2.

## [S1] Problem

When the local/ISP DNS is poisoned or broken for some trackers (typically Cloudflare-fronted private trackers), the only current workaround (`dns_overrides`) pins fixed IPs that go stale at every rotation. The user wants a configurable list of DNS servers; all of gowlarr's outbound resolution (dial + SSRF pre-check) goes through them, dynamically.

## [S2] Design

### Config

`Config.DNSOverrides map[string]string` is **removed**. `Config.DNSServers []string` (`json:"dns_servers,omitempty"`) is added: IPs or host:port (default 53); bracketed and bare IPv6 normalized to `[v6]:53`. Empty/omitted = system resolver (no behavior change). `Default()` leaves it empty. Old configs with `dns_overrides` load without error (unknown JSON keys ignored).

### netutil

New `internal/netutil/dnsservers.go`; `resolve.go`/`resolve_test.go` (OverrideDialContext + dnsOverrides atomic) are deleted.

- `SetDNSServers(servers []string)`: normalizes entries (skip blanks; strip `[`/`]` then `JoinHostPort(s, "53")` when `SplitHostPort` fails), stores a `*net.Resolver` behind `atomic.Pointer` (nil = system).
- `HasDNSServers() bool`; unexported `currentDNSServerResolver()`.
- `DialContextFor()`: **lazy** closure returning `(&net.Dialer{Resolver: currentDNSServerResolver()}).DialContext` (resolver looked up per dial).
- `newDNSServerResolver(addrs)`: `PreferGo: true`, `Dial` ignores the resolv.conf `address` param, forwards `network` (udp/tcp) unchanged (Go resolver handles UDP→TCP fallback), tries servers starting at a **round-robin offset** (`atomic.Uint64`, modulo before int cast to avoid overflow panic) with sequential failover on connect errors. Inner `net.Dialer` has no Resolver (no recursion if an entry is a hostname).
- `validate.go` `lookupHost`: uses `currentDNSServerResolver().LookupHost` when set, else `net.DefaultResolver` (IP literals already short-circuit before lookupHost).

### Wiring

- `cli/root.go` `openStore`: `netutil.SetDNSServers(cfg.DNSServers)` replaces `SetDNSOverrides`.
- `service/provider.go`: `applyDNSOverrides` → `applyDNSServers(client)` called unconditionally in `newIndexerHTTPClient` (early-return when `!HasDNSServers()`); clones transport, sets `DialContext = netutil.DialContextFor()`, `ForceAttemptHTTP2 = true`. **Guard: skip when `tr.DialContext != nil`** (SOCKS5 cardigann path — replacing would bypass the proxy). `clientOptions.applyDNS` removed. Call sites (`BuildConfiguredProviders`, `buildIndexerProvider`, `indexer.Test`) drop `applyDNS: true`.
- `newznab/client.go`: `NewWithOverrides`/`transportWithOverrides` deleted; new `New(id, name, baseURL, apiKey)` always clones via `cloneSharedTransport()` then `transportWithDNSServers`. `NewWithTransport` loses the `overrides` param; installs `transportWithDNSServers` on no-proxy and http/https-proxy paths (not socks5). `search.go` uses `New`.
- Download Cardigann path now honors the global `dns_servers` policy (deliberate change: it is a network policy like the proxy, not per-host pinning). Bare `defaultClient` fallback stays on system DNS.

### Scope limits (documented)

- Destinations behind HTTP/SOCKS5 proxies are resolved by the proxy; SOCKS5 proxy hostname itself uses system DNS.
- Failover is connect-level (UDP connect succeeds to blackholed servers); round-robin spreads first tries; dead servers surface as DNS exchange timeouts.
- `PreferGo` bypasses cgo; `/etc/hosts` still consulted first.
- No per-domain routing (`Resolver.Dial` does not receive the hostname). No DoH/DoT. cardigann-go unchanged.

## [S3] Out of Scope

- cardigann-go changes (no Options hook, no release).
- Per-domain DNS routing.
- DoH/DoT.
- `dns_overrides` migration/deprecation (removed outright; old key silently ignored).
- SOCKS5 proxy hostname resolution via `dns_servers`.
- FlareSolverr.
- `config show` surfacing `dns_servers`.

## Tasks

- [x] T1: config — remove DNSOverrides, add DNSServers — acceptance: round-trip `dns_servers`; old `dns_overrides` ignored without error; `Default()` empty (covers: S2)
- [x] T2: netutil — dnsservers.go (Set/Has/DialContextFor lazy/normalize strip-brackets/round-robin); purge resolve.go; lookupHost via resolver — acceptance: fake UDP packet received via SetDNSServers and via `lookupHost` var; round-robin offsets advance; IPv4/IPv6 bracketed/bare/port normalization; system behavior without servers (covers: S2; depends: —)
- [x] T3: cli — SetDNSServers in openStore — acceptance: wiring correct (covers: S2; depends: T1, T2)
- [x] T4: service — applyDNSServers with DialContext!=nil guard; remove applyDNS; comment sweep — acceptance: dns_servers active (no-proxy and http-proxy) → DialContext+ForceAttemptHTTP2; none → shared transport untouched; pre-existing DialContext (socks5) preserved; download Cardigann covered; no dns_overrides mention left in service (covers: S2; depends: T2)
- [x] T5: newznab — New replaces NewWithOverrides (always clone); NewWithTransport without overrides; transportWithDNSServers (not socks5); 9 call sites + bench; new DNSServers test — acceptance: no-proxy+dns_servers → DialContext; socks5+dns_servers → socks5 DialContext preserved; TestNew_TransportsForceHTTP2 green; go test compiles incl. bench; no DNS overrides mention in newznab (covers: S2; depends: T2)
- [x] T6: search.go — NewWithOverrides → New — acceptance: build green, ad-hoc newznab honors global dns_servers (covers: S2; depends: T5)
- [x] T7: README — document dns_servers — acceptance: subsection present, wording faithful to implementation (covers: S2)
- [x] T8: verification — make test (race), make lint, make build — acceptance: all green (covers: S2; depends: T1–T7)
