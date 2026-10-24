# wpstatic

A Go binary that turns a local WordPress site into a **minified, compressed static cache** served by nginx. It crawls your site exactly as an anonymous visitor sees it, **strips out the sensitive WordPress surface** (admin, login, REST API, plugin internals, fingerprinting meta), minifies HTML/CSS/JS, writes gzip siblings for `gzip_static`, and publishes the result **atomically** so nginx never serves a half-written site.

Builds are triggered two ways:

- **Cron** — run `wpstatic build` on a schedule.
- **WordPress "go live" plugin** — a bundled single-file plugin adds a *Go Live* button (admin bar + settings page) that calls a localhost hook endpoint to rebuild on demand.

```
┌────────────┐   anonymous GET    ┌──────────────────────────────┐
│  WordPress │ ◄───────────────── │  wpstatic (Go)               │
│  (origin)  │                    │  crawl → strip → minify → gz │
└────────────┘                    └──────────────┬───────────────┘
                                                 │ atomic publish
                                                 ▼
                                        /var/www/wpstatic/live
                                                 │
                                                 ▼
┌────────────┐   static hit     ┌──────────────────────────────┐
│   nginx    │ ───────────────► │  live/  (index.html, .gz, …) │
│            │   dynamic miss   │  build-info.json             │
│            │ ───────────────► │  @dynamic → proxy to WP      │
└────────────┘                  └──────────────────────────────┘
```

## Why

- **Fast front door** — static HTML/CSS/JS + pre-gzipped assets are cheap to serve.
- **Smaller attack surface** — the public cache never contains `/wp-admin/`, `/wp-login.php`, `/wp-json/`, `xmlrpc.php`, or WordPress version fingerprints.
- **Safe deploys** — the live tree is swapped atomically; a failed build leaves the previous site intact.
- **Dynamic fallback** — anything not cached (search, comments, forms) transparently proxies to the real WordPress backend.

## How it works

1. **Crawl** — breadth-first from the homepage as an anonymous user (no cookies). Same-host only. Extracts pages (`<a href>`, meta refresh) and assets (`<link>`, `<script>`, `<img src/srcset>`, `<source>`, `<video>/<audio>`, CSS `url()`). Blocked paths are never fetched.
2. **Strip** — removes links/forms/scripts pointing at sensitive paths, the `<meta name="generator">` tag, pingback links, the admin bar (if ever rendered), and tracking query params. Same-host absolute URLs are rewritten to relative so the cache is host-agnostic.
3. **Minify** — HTML/CSS/JS via [tdewolff/minify](https://github.com/tdewolff/minify). Best-effort: on any minify error the original bytes are kept.
4. **Compress** — writes a `.gz` sibling for every file so nginx can use `gzip_static`.
5. **Publish** — builds into `<output.dir>/.staging-<id>`, then atomically renames it to `<output.dir>/live`, removing the previous tree. `build-info.json` records the build.

## Install

```sh
git clone https://github.com/tekmanic/wpstatic
cd wpstatic
make build          # → bin/wpstatic
sudo make install   # → /usr/local/bin/wpstatic
```

Requires Go 1.22+ to build. The binary is a single static-ish executable (CGO not required).

## Quick start

```sh
# 1. Generate a config
wpstatic config init            # writes ./wpstatic.yaml

# 2. Point it at your site + output dir (edit wpstatic.yaml, or use env)
export WPSTATIC_SITE_URL=http://localhost:8081
export WPSTATIC_OUTPUT_DIR=/var/www/wpstatic

# 3. Build
wpstatic build
# build 3f9a… complete: 42 pages, 187 assets, 3.2MB in 4.1s (skipped 0, errors 0)

# 4. Point nginx at /var/www/wpstatic/live (see examples/nginx.conf)
```

## Commands

| Command | Description |
|---|---|
| `wpstatic build` | Crawl the site and publish a new static build |
| `wpstatic hook` | Run the localhost trigger endpoint for the WP plugin |
| `wpstatic config init` | Write a sample `wpstatic.yaml` |
| `wpstatic config get [key]` | Print a config value (or all) |
| `wpstatic config set <key> <value>` | Set a value in the config file |
| `wpstatic version` | Print version info |

Global flags: `-c, --config <path>` (default `$WPSTATIC_CONFIG` or `./wpstatic.yaml`), `-v, --verbose`.

## Configuration

Precedence: **CLI flag > environment > config file > default**. Env vars use the `WPSTATIC_` prefix with `.` → `_` (e.g. `WPSTATIC_SITE_URL`, `WPSTATIC_HOOK_SECRET`).

```yaml
site:
  url: http://localhost:8081        # WordPress origin to scrape

output:
  dir: /var/www/wpstatic            # live tree published to <dir>/live

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
  # secret: set via WPSTATIC_HOOK_SECRET (required when enabled)

user_agent: wpstatic/1.0 (+local static cache)
```

### What gets stripped (defaults)

**Blocked paths** (never fetched or crawled, and removed from HTML):
`/wp-admin/`, `/wp-login.php`, `/wp-json/`, `/xmlrpc.php`, `/wp-cron.php`, `/wp-activate.php`, `/wp-signup.php`, `/wp-links-opml.php`, `/wp-trackback.php`, `/wp-comments-post.php`, `/wp-mail.php`, `/wp-pass.php`, `/wp-register.php`, `/wp-settings.php`, `/readme.html`, `/license.txt`, `/wp-content/mu-plugins/`, `/wp-content/debug.log`, `/wp-config.php`/`/wp-config.*`, `/.env`, `/.git/`, `/.svn/`, `/.htaccess`, and a few `wp-includes` internals.

**Tracking query params** (stripped from every URL): `utm_*`, `fbclid`, `gclid`, `ref`, `replytocom`, `cpage`, `_wpnonce`, `_wp_http_referer`, `share`, `add-to-cart`, `orderby`, `order`, `product-page`.

**Forms**: with `strip_forms: wp-endpoints` (default), forms whose action resolves to a blocked path are removed (e.g. comment forms posting to `/wp-comments-post.php`). `all` removes every form; `none` keeps them.

## The WordPress "go live" plugin

A single-file plugin at `plugin/wpstatic-golive/wpstatic-golive.php`:

1. Copy it into your theme or a plugins directory:
   ```sh
   cp plugin/wpstatic-golive/wpstatic-golive.php /path/to/wp-content/plugins/wpstatic-golive.php
   ```
2. Activate it in WordPress.
3. Go to **Settings → wpstatic Go Live** and set the **Hook URL** (`http://127.0.0.1:9090`) and **Hook secret**.
4. Run the hook server on the host:
   ```sh
   export WPSTATIC_HOOK_SECRET='a-long-random-string'
   wpstatic hook
   ```
5. Click **Go Live** in the admin bar (or **Go Live Now** on the settings page). The plugin POSTs to `/go-live` with the shared secret; wpstatic rebuilds and publishes. `GET /status` reports `idle | running | success | error`.

The hook binds to `127.0.0.1` by default and requires the secret (constant-time compare), so it's not reachable off the host.

## nginx

See [`examples/nginx.conf`](examples/nginx.conf). Key points:

- `root /var/www/wpstatic/live;`
- `gzip_static on;` — serves the pre-compressed `.gz` siblings.
- `try_files $uri $uri/index.html $uri.html @dynamic;` — static hit, else fall back.
- `@dynamic { proxy_pass http://127.0.0.1:8081; }` — uncached requests go to the real WordPress backend.

## Cron

```cron
# Rebuild every 15 minutes
*/15 * * * * cd /opt/wpstatic && /usr/local/bin/wpstatic build >> /var/log/wpstatic.log 2>&1
```

Or use the plugin for on-demand rebuilds after content changes.

## Security notes

- The crawler makes **no authenticated requests** — it sees only the anonymous view, so the admin bar and logged-in content never enter the cache.
- The hook endpoint is **localhost-only** and **secret-gated**.
- Secrets (hook secret) should come from the environment, never the config file.
- The static cache is a **snapshot** — it does not hide the dynamic backend. If you want the backend fully private, put it behind auth/VPN and let nginx be the only public entry point.

## Limitations

- **Query-string pages** (e.g. `/?p=123` on non-pretty permalinks, search results) can't map to a clean static path and are skipped; they fall through to the dynamic backend via nginx.
- **Dynamic behavior** (search, comments, carts, personalization) is not replicated — it falls back to WordPress.
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
cmd/            Cobra CLI (build, hook, config, version)
internal/config   Viper config + validation
internal/crawl    BFS crawler, URL/asset extraction, concurrency
internal/strip    sensitive-content removal + URL rewriting
internal/minify   tdewolff minify + gzip
internal/publish  atomic staging→live swap
internal/hook     localhost trigger endpoint
plugin/           WordPress "go live" plugin (PHP)
examples/         nginx.conf
```

## License

MIT (see [LICENSE](LICENSE)).
