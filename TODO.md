# TODO — wpresto

Resumable work breakdown for the WordPress → static cache binary.
Last updated: 2026-10-24.

## Definition of done
A single Go binary that crawls a local WordPress site as an anonymous user,
strips sensitive admin/login/API/plugin content, minifies + gzips the result,
and publishes it atomically to a static filesystem served by nginx — triggerable
by cron or by a WordPress "go live" plugin.

## Status: ✅ COMPLETE (core) — pending: manual smoke test on a real WP host

---

## Phase 1 — Scaffold ✅
- [x] Go module `github.com/tekmanic/wpresto` (go 1.22)
- [x] Cobra/Viper CLI skeleton
- [x] Package layout: cmd, internal/{config,crawl,strip,minify,publish,hook}, plugin, examples

## Phase 2 — Config ✅
- [x] Viper: flag > env (WPRESTO_*) > file > default
- [x] Sections: site, output, crawl, strip, minify, compress, hook, user_agent
- [x] Validation (URL scheme, concurrency, timeout, strip_forms enum, hook secret required when enabled)
- [x] `config init/get/set` commands
- [x] Tests: defaults, validation, file load, env override, missing file

## Phase 3 — Crawler ✅
- [x] BFS from homepage, anonymous (no cookies), same-host only
- [x] Concurrency (worker pool, cond-var queue, no deadlock)
- [x] Page extraction: `<a href>`, meta refresh
- [x] Asset extraction: `<link>`, `<script>`, `<img src/srcset>`, `<source>`, `<video>/<audio>`, `<embed>`, CSS `url()`
- [x] Blocked paths never fetched (blocklist filter in enqueue)
- [x] MaxPages cap, per-request timeout, max asset size
- [x] URL→path mapping: `/`→index.html, `/foo`→foo/index.html, `/x.html`→x.html; assets keep original layout
- [x] Query-string pages skipped (can't map cleanly)
- [x] Tests: path mapping, resolve, srcset, CSS refs, end-to-end (httptest)

## Phase 4 — Stripper ✅
- [x] Remove links/forms/scripts/imgs pointing at blocked paths
- [x] Remove `<meta name="generator">` (configurable)
- [x] Remove pingback links, admin bar (defensive)
- [x] Strip tracking query params from all URLs
- [x] Rewrite same-host absolute URLs → relative (host-agnostic cache)
- [x] Form policy: all | wp-endpoints | none
- [x] Tests: sensitive links, generator meta, pingback, forms, tracking params, relative rewrite, IsBlocked, CleanQuery

## Phase 5 — Minify + Compress ✅
- [x] tdewolff/minify for HTML/CSS/JS (best-effort fallback to original)
- [x] Per-type toggles (html/css/js) + master enable
- [x] gzip siblings (.gz) for nginx gzip_static
- [x] Tests: minify each type, disabled passthrough, unknown mime, gzip round-trip, WriteFile

## Phase 6 — Publisher ✅
- [x] Build into `<dir>/.staging-<id>`
- [x] Atomic rename to `<dir>/live` (previous tree moved aside, then removed)
- [x] Rollback on failed swap
- [x] build-info.json (id, times, duration, pages, assets, bytes, site)
- [x] Tests: promote, replace previous, tree bytes

## Phase 7 — Hook server ✅
- [x] localhost HTTP server (default 127.0.0.1:9090)
- [x] POST /go-live with X-WPresto-Secret (constant-time compare)
- [x] 202 accepted / 409 busy / 401 unauthorized
- [x] GET /status (idle|running|success|error), GET /healthz
- [x] Async build, single-flight
- [x] Tests: secret required, triggers build, conflict when busy, healthz

## Phase 8 — CLI ✅
- [x] build, hook, config init/get/set, version
- [x] Global --config / --verbose
- [x] RunBuild orchestrator (staging → crawl → publish, cleanup on failure)
- [x] End-to-end cmd test (httptest fake WP → live output verified)

## Phase 9 — WordPress plugin ✅
- [x] Single-file plugin `plugin/wpresto-golive/wpresto-golive.php`
- [x] Settings page (hook URL + secret), admin bar "Go Live", "Go Live Now" button
- [x] AJAX trigger (nonce + manage_options), server-side secret (client values ignored)
- [x] Escaped output, i18n, PHP 7.4 compatible
- [x] Delegates: written by subagent, reviewed by Radix

## Phase 10 — Docs + CI ✅
- [x] README.md (architecture, install, config, plugin, nginx, cron, security, limitations)
- [x] examples/nginx.conf (gzip_static + dynamic fallback)
- [x] Makefile, .gitignore, LICENSE (MIT)
- [x] .github/workflows/ci.yml (vet, gofmt, test -race, build, version smoke)

## Phase 11 — Verification ✅
- [x] go build ./... clean
- [x] go vet ./... clean
- [x] gofmt -l . clean
- [x] go test -race -count=1 ./... — all packages pass
- [x] CLI smoke: version, --help

## Phase 12 — GitHub ✅
- [x] Repo tekmanic/wpresto created
- [x] Branch pushed
- [x] PR opened

## Phase 13 — Plugin system (dynamic parts) ✅
- [x] `internal/plugin` — Lambda-style `Handler` (`func(ctx, *http.Request) (*http.Response, error)`),
      `Plugin` interface (Name/Description/Kinds/Routes/Handle), `FuncPlugin` adapter
- [x] Registry: `Register`/`All`/`Get`/`FindKind`/`Find`; exact + prefix (`*`) route matching
- [x] Simple server (`wpresto serve`): plugin routes run the handler; unmatched requests
      proxy to the WordPress fallback (config `plugins.fallback`)
- [x] `internal/detect` — scans crawled HTML for dynamic parts, classifies:
      forms → contact-form / subscribe / comment-form / search;
      scripts/API → analytics / geoip / api / ajax
- [x] `inventory/` — 5 example plugins compiled into the binary (each a Go package,
      self-registering in `init()`): contactform, subscribe, geoip, analytics, urlrewrite
- [x] Build wiring: detection runs during crawl; matched form actions rewritten to
      plugin routes (`strip.Options.RewriteFormAction`); `live/dynamic-report.json`
      written; summary line `dynamic: N detected, M handled, K flagged`
- [x] Unmatched dynamic parts are FLAGGED in the report (no plugin → visible gap)
- [x] Config: `plugins` section (enabled, listen, fallback); `serve` command
- [x] Docs: README plugins section, examples/nginx.conf (plugin server as single
      dynamic entry point)
- [x] Tests: registry/server/matching, detect classification, strip form-rewrite,
      end-to-end build dynamic-report
- [x] Smoke: `wpresto serve` live — contact 200, validation 422, geoip 200,
      subscribe 200, analytics 204, rewrite 304/404, unmatched 502 (fallback down)

---

## Open / follow-up
- [ ] **Manual smoke test on a real WordPress host**: run `wpresto build` against a
      live WP site, verify the static output in a browser, confirm admin/login/API
      are absent, and confirm the dynamic fallback works. (Needs a host with WP + nginx.)
- [ ] **Optional**: Dockerfile for the binary (single static binary, easy to ship).
- [ ] **Optional**: `wpresto build --dry-run` (crawl + report, no publish).
- [ ] **Optional**: sitemap.xml generation from the crawled page set.
- [ ] **Optional**: respect robots.txt (currently ignored — local site assumption).
- [ ] **Optional**: image optimization (resize/compress) beyond passthrough.

## Design decisions
1. **Same-host only** — external/CDN assets are referenced, not downloaded. Keeps the
   cache self-contained for the site's own assets and avoids pulling third-party content.
2. **Query-string pages skipped** — they can't map to a clean static path; nginx's
   `@dynamic` fallback covers them.
3. **Best-effort minify** — a file that fails to minify is served unminified rather than
   failing the whole build.
4. **Atomic publish** — staging dir + rename; a failed build never corrupts the live site.
5. **Anonymous crawl** — no cookies/auth, so the cache reflects the public view only.
6. **Hook is localhost + secret** — the trigger endpoint is not publicly reachable.
7. **CSS url() not rewritten** — assets keep their original layout, so relative url()
   references resolve identically on the static site; we only need to download them.

## Risks
- **Large sites** — MaxPages + concurrency bound memory/time; tune for very large sites.
- **Frequent content changes** — cron interval vs. freshness trade-off; use the plugin
  for on-demand rebuilds after publishes.
- **Dynamic features** — search/comments/carts always fall back to WordPress; the static
  cache is a front door, not a replacement.
- **Heuristic detection** — dynamic-part classification is based on form classes/ids/
  field names and script patterns; unusual markup may be flagged rather than matched.
  Flagged parts are visible in `dynamic-report.json` — that's the intended safety net.
