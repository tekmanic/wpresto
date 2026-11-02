# wpresto

**wpresto** is an open-source WordPress-to-static exporter that transforms your WordPress site into portable, deployable static files.

Under the hood it's a single Go binary that turns a local WordPress site into a **minified, compressed static cache** served by nginx. It crawls your site exactly as an anonymous visitor sees it, **strips out the sensitive WordPress surface** (admin, login, REST API, plugin internals, fingerprinting meta), minifies HTML/CSS/JS, writes gzip siblings for `gzip_static`, and publishes the result **atomically** so nginx never serves a half-written site.

On top of the static cache, wpresto has a **plugin system** that replaces the *dynamic* parts of your site. During a build it **detects dynamic content** (contact forms, subscribe forms, analytics beacons, API/AJAX endpoints), **matches each to a plugin** in the inventory, **rewrites the form/endpoint to point at the plugin**, and **flags anything no plugin covers**. A small `wpresto serve` process runs the matched plugins in a simple HTTP server (Lambda-style handlers), so the static site stays dynamic-capable without the WordPress backend.

Builds are triggered two ways:

- **Cron** — run `wpresto build` on a schedule.
- **WordPress "go live" plugin** — a bundled single-file plugin adds a *Go Live* button (admin bar + settings page) that calls a localhost hook endpoint to rebuild on demand.

```
┌────────────┐   anonymous GET    ┌───────────────────────────────────┐
│  WordPress │ ◄───────────────── │  wpresto build (Go)              │
│  (origin)  │                    │  crawl → detect → strip → minify │
└────────────┘                    │  → gz → match plugins → publish  │
                                  └──────────────┬────────────────────┘
                                                 │ atomic publish
                                                 ▼
                                        /var/www/wpresto/live
                                        (index.html, .gz, …,
                                         dynamic-report.json)
                                                 │
                                                 ▼
┌────────────┐   static hit     ┌───────────────────────────────────┐
│   nginx    │ ───────────────► │  live/  (served directly)         │
│            │   /wpresto/*     │                                   │
│            │ ───────────────► │  wpresto serve (plugin server)    │
│            │   dynamic miss   │   plugin route → Lambda handler   │
│            │ ───────────────► │   unmatched  → proxy to WP        │
└────────────┘                  └───────────────────────────────────┘
```

## Why

- **Fast front door** — static HTML/CSS/JS + pre-gzipped assets are cheap to serve.
- **Smaller attack surface** — the public cache never contains `/wp-admin/`, `/wp-login.php`, `/wp-json/`, `xmlrpc.php`, or WordPress version fingerprints.
- **Safe deploys** — the live tree is swapped atomically; a failed build leaves the previous site intact.
- **Dynamic-capable** — a plugin system detects dynamic parts (forms, analytics, endpoints), rewrites them to Lambda-style plugin handlers, and flags what's uncovered.
- **Dynamic fallback** — anything not cached and not covered by a plugin transparently proxies to the real WordPress backend.

## How it works

1. **Crawl** — breadth-first from the homepage as an anonymous user (no cookies). Same-host only. Extracts pages (`<a href>`, meta refresh) and assets (`<link>`, `<script>`, `<img src/srcset>`, `<source>`, `<video>/<audio>`, CSS `url()`). Blocked paths are never fetched.
2. **Detect** — scans each page for dynamic parts (forms, analytics/tracking scripts, API/AJAX endpoints) and classifies them (contact-form, subscribe, comment-form, search, analytics, geoip, api, ajax).
3. **Strip** — removes links/forms/scripts pointing at sensitive paths, the `<meta name="generator">` tag, pingback links, the admin bar (if ever rendered), and tracking query params. Same-host absolute URLs are rewritten to relative so the cache is host-agnostic. A form whose class matches a plugin is **kept and its action rewritten** to the plugin route instead of being stripped.
4. **Minify** — HTML/CSS/JS via [tdewolff/minify](https://github.com/tdewolff/minify). Best-effort: on any minify error the original bytes are kept.
5. **Compress** — writes a `.gz` sibling for every file so nginx can use `gzip_static`.
6. **Match** — each detected dynamic part is matched to a plugin by class. Matched parts are served by the plugin; unmatched parts are **flagged** in the report.
7. **Publish** — builds into `<output.dir>/.staging-<id>`, then atomically renames it to `<output.dir>/live`, removing the previous tree. `build-info.json` records the build and `dynamic-report.json` records the detection/matching outcome.

## Install

```sh
git clone https://github.com/tekmanic/wpresto
cd wpresto
make build          # → bin/wpresto
sudo make install   # → /usr/local/bin/wpresto
```

Requires Go 1.22+ to build. The binary is a single static-ish executable (CGO not required).

## Quick start

```sh
# 1. Generate a config
wpresto config init            # writes ./wpresto.yaml

# 2. Point it at your site + output dir (edit wpresto.yaml, or use env)
export WPRESTO_SITE_URL=http://localhost:8081
export WPRESTO_OUTPUT_DIR=/var/www/wpresto

# 3. Build
wpresto build
# build 3f9a… complete: 42 pages, 187 assets, 3.2MB in 4.1s (skipped 0, errors 0)

# 4. Point nginx at /var/www/wpresto/live (see examples/nginx.conf)
```

## Commands

| Command | Description |
|---|---|
| `wpresto build` | Crawl the site, detect dynamic parts, and publish a new static build |
| `wpresto serve` | Run the plugin server that replaces dynamic parts (Lambda-style handlers) |
| `wpresto hook` | Run the localhost trigger endpoint for the WP plugin |
| `wpresto config init` | Write a sample `wpresto.yaml` |
| `wpresto config get [key]` | Print a config value (or all) |
| `wpresto config set <key> <value>` | Set a value in the config file |
| `wpresto version` | Print version info |

Global flags: `-c, --config <path>` (default `$WPRESTO_CONFIG` or `./wpresto.yaml`), `-v, --verbose`.

## Configuration

Precedence: **CLI flag > environment > config file > default**. Env vars use the `WPRESTO_` prefix with `.` → `_` (e.g. `WPRESTO_SITE_URL`, `WPRESTO_HOOK_SECRET`).

```yaml
site:
  url: http://localhost:8081        # WordPress origin to scrape

output:
  dir: /var/www/wpresto            # live tree published to <dir>/live

crawl:
  concurrency: 8                    # parallel fetches
  timeout: 30s                      # per-request timeout
  max_pages: 500                    # safety cap on HTML pages
  max_asset_bytes: 20971520         # 20MB cap per asset

strip:
  strip_forms: wp-endpoints         # all | wp-endpoints | none
  strip_generator_meta: true        # remove <meta name="generator">
  # blocklist: [...]                # override default sensitive paths
  # query_params: [...]             # override default tracking params

minify:
  enabled: true
  html: true
  css: true
  js: true

compress:
  gzip: true                        # write .gz siblings for gzip_static

hook:
  enabled: false
  listen: 127.0.0.1:9090
  # secret: set via WPRESTO_HOOK_SECRET (required when enabled)

plugins:
  enabled: true                     # detect dynamic parts + run the plugin server
  listen: 127.0.0.1:9091            # wpresto serve bind address
  # fallback: http://127.0.0.1:8081 # unmatched requests proxy here (default: site.url)

user_agent: wpresto/1.0 (+local static cache)
```

### What gets stripped (defaults)

**Blocked paths** (never fetched or crawled, and removed from HTML):
`/wp-admin/`, `/wp-login.php`, `/wp-json/`, `/xmlrpc.php`, `/wp-cron.php`, `/wp-activate.php`, `/wp-signup.php`, `/wp-links-opml.php`, `/wp-trackback.php`, `/wp-comments-post.php`, `/wp-mail.php`, `/wp-pass.php`, `/wp-register.php`, `/wp-settings.php`, `/readme.html`, `/license.txt`, `/wp-content/mu-plugins/`, `/wp-content/debug.log`, `/wp-config.php`/`/wp-config.*`, `/.env`, `/.git/`, `/.svn/`, `/.htaccess`, and a few `wp-includes` internals.

**Tracking query params** (stripped from every URL): `utm_*`, `fbclid`, `gclid`, `ref`, `replytocom`, `cpage`, `_wpnonce`, `_wp_http_referer`, `share`, `add-to-cart`, `orderby`, `order`, `product-page`.

**Forms**: with `strip_forms: wp-endpoints` (default), forms whose action resolves to a blocked path are removed (e.g. comment forms posting to `/wp-comments-post.php`). `all` removes every form; `none` keeps them.

## The WordPress "go live" plugin

A single-file plugin at `plugin/wpresto-golive/wpresto-golive.php`:

1. Copy it into your theme or a plugins directory:
   ```sh
   cp plugin/wpresto-golive/wpresto-golive.php /path/to/wp-content/plugins/wpresto-golive.php
   ```
2. Activate it in WordPress.
3. Go to **Settings → wpresto Go Live** and set the **Hook URL** (`http://127.0.0.1:9090`) and **Hook secret**.
4. Run the hook server on the host:
   ```sh
   export WPRESTO_HOOK_SECRET='a-long-random-string'
   wpresto hook
   ```
5. Click **Go Live** in the admin bar (or **Go Live Now** on the settings page). The plugin POSTs to `/go-live` with the shared secret; wpresto rebuilds and publishes. `GET /status` reports `idle | running | success | error`.

The hook binds to `127.0.0.1` by default and requires the secret (constant-time compare), so it's not reachable off the host.

## Plugins (dynamic parts)

The plugin system replaces the *dynamic* parts of your site so the static cache
stays functional without the WordPress backend.

**How it works:**

1. **Detect** — during `wpresto build`, every crawled page is scanned for
   dynamic parts: `<form>` elements (classified as contact-form, subscribe,
   comment-form, or search) and `<script src>` / API references (analytics,
   geoip, api, ajax).
2. **Match** — each detected part is matched to a plugin by class. A plugin
   declares the classes it handles via `Kinds()`.
3. **Rewrite** — a form whose class matches a plugin is kept and its `action`
   is rewritten to the plugin's route (e.g. `/wpresto/contact`), so the static
   form posts to the plugin server instead of WordPress.
4. **Flag** — a part no plugin covers is **flagged** in the build output and in
   `live/dynamic-report.json`. It falls back to WordPress via nginx.
5. **Serve** — `wpresto serve` runs the matched plugins in a simple HTTP server
   (Lambda-style handlers). nginx points its dynamic fallback at this server.

**The inventory** (`inventory/`) is a set of Go packages, each a plugin written
in AWS Lambda Go HTTP-handler style:

```go
func Handler(ctx context.Context, req *http.Request) (*http.Response, error)
```

The exact signature the AWS Lambda runtime uses for API Gateway HTTP proxy
integrations — the same handler code deploys unchanged to Lambda. Each plugin
registers itself in `init()` and is compiled into the wpresto binary. Adding a
plugin: create `inventory/<name>/<name>.go`, then add a blank import in
`inventory/inventory.go`.

**Bundled plugins:**

| Plugin | Route | Replaces |
|---|---|---|
| `contact-form` | `POST /wpresto/contact` | WordPress contact forms (CF7, etc.) |
| `subscribe` | `POST /wpresto/subscribe` | Newsletter/subscribe forms |
| `geoip` | `GET /wpresto/geoip` | Client geolocation (stub — wire a GeoIP DB) |
| `analytics` | `POST /wpresto/analytics` | Third-party analytics beacons (GA/GTM) |
| `urlrewrite` | `GET /wpresto/rewrite/*` | Legacy/changed URL redirects |

**Running the plugin server:**

```sh
wpresto config set plugins.enabled true
wpresto config set plugins.listen 127.0.0.1:9091
# (optional) wpresto config set plugins.fallback http://127.0.0.1:8081
wpresto serve
```

The server lists the registered plugins on startup, answers plugin routes, and
proxies unmatched requests to the fallback (WordPress). Run it under systemd.

**The build report** (`live/dynamic-report.json`) records, per build:

```json
{
  "detected": 3,
  "handled": 2,
  "flagged": 1,
  "parts": [
    {"part": {"page": "/contact/", "kind": "form", "class": "contact-form", "target": "/wp-admin/admin-post.php"}, "plugin": "contact-form", "handled": true},
    {"part": {"page": "/blog/", "kind": "form", "class": "comment-form", "target": "/wp-comments-post.php"}, "handled": false}
  ]
}
```

`flagged` parts are the ones you need a plugin for — build one in `inventory/`
and the next build will pick it up.

## nginx

See [`examples/nginx.conf`](examples/nginx.conf). Key points:

- `root /var/www/wpresto/live;`
- `gzip_static on;` — serves the pre-compressed `.gz` siblings.
- `try_files $uri $uri/index.html $uri.html @dynamic;` — static hit, else fall back.
- `location /wpresto/ { proxy_pass http://wpresto_plugins; }` — plugin endpoints.
- `@dynamic { proxy_pass http://wpresto_plugins; }` — the plugin server answers plugin routes and proxies everything else to the WordPress backend.

The plugin server (`wpresto serve`) is the single dynamic entry point, so nginx
never talks to WordPress directly.

## Cron

```cron
# Rebuild every 15 minutes
*/15 * * * * cd /opt/wpresto && /usr/local/bin/wpresto build >> /var/log/wpresto.log 2>&1
```

Or use the plugin for on-demand rebuilds after content changes.

## Security notes

- The crawler makes **no authenticated requests** — it sees only the anonymous view, so the admin bar and logged-in content never enter the cache.
- The hook endpoint is **localhost-only** and **secret-gated**.
- Secrets (hook secret) should come from the environment, never the config file.
- The static cache is a **snapshot** — it does not hide the dynamic backend. If you want the backend fully private, put it behind auth/VPN and let nginx be the only public entry point.

## Limitations

- **Query-string pages** (e.g. `/?p=123` on non-pretty permalinks, search results) can't map to a clean static path and are skipped; they fall through to the dynamic backend via nginx.
- **Dynamic behavior** is replaced only where a plugin exists. Contact forms, subscribe forms, analytics, geoip, and URL rewrites are covered by the bundled plugins; anything else (search, comments, carts, personalization) is **flagged** in the build report and falls back to WordPress. Add a plugin in `inventory/` to cover more.
- **Detection is heuristic** — forms are classified by class/id/field names. Unusual markup may be classified as a generic `form` and flagged rather than matched.
- **External/CDN assets** are referenced but not downloaded (same-host only).
- Minification is best-effort; a file that fails to minify is served unminified.

## Development

```sh
make build    # build
make test     # go test -race ./...
make vet      # go vet
make fmt      # gofmt -w
```

Layout:

```
cmd/            Cobra CLI (build, serve, hook, config, version)
internal/config   Viper config + validation
internal/crawl    BFS crawler, URL/asset extraction, concurrency
internal/detect   dynamic-part detection + classification
internal/strip    sensitive-content removal + URL rewriting + form→plugin rewrite
internal/minify   tdewolff minify + gzip
internal/publish  atomic staging→live swap
internal/hook     localhost trigger endpoint
internal/plugin   plugin interface, registry, matcher, simple HTTP server
inventory/        the plugin inventory (contact-form, subscribe, geoip, analytics, urlrewrite)
plugin/           WordPress "go live" plugin (PHP)
examples/         nginx.conf
```

## License

MIT (see [LICENSE](LICENSE)).
