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

## Compression

thumb doesn't compress on the fly: it serves pre-compressed copies written by the build next to
the files (`app.js.br`, `app.js.gz`), e.g. with `gzip -k -9` / `brotli -k`, or a Vite plugin.

- If the client's `Accept-Encoding` allows it, the `.br` copy is sent, else the `.gz` one, with
  `Content-Encoding` and the original file's `Content-Type`. Otherwise the plain file is sent.
- A file that has a copy always gets `Vary: Accept-Encoding`.
- `Cache-Control` applies to compressed responses the same way.
- Files with an unknown extension (no known `Content-Type`) and directory paths such as `/` are
  always sent plain.
