# THUMB - Static file server

Serves a single-page app: `assets/*` as files, any other file next to `index.html`, and
`index.html` for every other path.

```bash
./thumb [flags] <path/to/index.html> <port>
```

Flags (both optional, no header when left out):

| flag | sets `Cache-Control` on | typical value |
| --- | --- | --- |
| `-index-cache` | `index.html` and the other files outside `assets/` | `no-cache` (a redeploy is picked up on the next load) |
| `-assets-cache` | the files under `assets/` that exist (never on a 404) | `public, max-age=31536000, immutable` (hashed file names) |

```bash
./thumb -index-cache "no-cache" -assets-cache "public, max-age=31536000, immutable" dist/index.html 8061
```
