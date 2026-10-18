// thumb serves a single-page app: files under assets/ as they are, any other file that exists next
// to index.html, and index.html itself for every other path, so the app's router handles the
// address. Optional flags set Cache-Control headers: typically "no-cache" on the index, so a
// redeploy is picked up on the next load, and a long "immutable" on the hashed assets.
// A file with a pre-compressed copy next to it (file.br, file.gz) is sent compressed to the
// clients that accept it.
package main

import (
	"flag"
	"fmt"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// encoding is a pre-compressed copy of a file: the file's name + ext, sent with "Content-Encoding: coding".
type encoding struct{ coding, ext string }

// filePath is the path on disk of urlPath under root.
func filePath(root, urlPath string) string {
	return filepath.Join(root, filepath.Clean("/"+urlPath))
}

// exists reports whether urlPath names a file under root.
func exists(root, urlPath string) bool {
	_, err := os.Stat(filePath(root, urlPath))
	return err == nil
}

// isRegular reports whether name is a regular file.
func isRegular(name string) bool {
	info, err := os.Stat(name)
	return err == nil && info.Mode().IsRegular()
}

// withCacheControl sets the Cache-Control header to value on every response of next; an empty value sets none.
func withCacheControl(value string, next http.Handler) http.Handler {
	if value == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", value)
		next.ServeHTTP(w, r)
	})
}

// withFileCacheControl is withCacheControl for files that exist under root only: a missing file's
// 404 must not be remembered for a year.
func withFileCacheControl(value, root string, next http.Handler) http.Handler {
	if value == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if exists(root, r.URL.Path) {
			w.Header().Set("Cache-Control", value)
		}
		next.ServeHTTP(w, r)
	})
}

// zeroQ reports whether the parameters of an Accept-Encoding entry hold q=0 (the coding is refused).
func zeroQ(params string) bool {
	for _, param := range strings.Split(params, ";") {
		key, value, _ := strings.Cut(param, "=")
		if strings.EqualFold(strings.TrimSpace(key), "q") {
			q, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
			return err == nil && q == 0
		}
	}
	return false
}

// accepts reports whether an Accept-Encoding header allows coding: listed, or covered by "*", without q=0.
func accepts(header, coding string) bool {
	star := false
	for _, entry := range strings.Split(header, ",") {
		token, params, _ := strings.Cut(entry, ";")
		token = strings.ToLower(strings.TrimSpace(token))
		if token == coding {
			return !zeroQ(params)
		}
		if token == "*" {
			star = !zeroQ(params)
		}
	}
	return star
}

// compressedCopies returns the encodings of encs that name has a pre-compressed copy for, in encs order.
func compressedCopies(name string, encs []encoding) []encoding {
	var found []encoding
	if name == "" {
		return found
	}
	for _, enc := range encs {
		if isRegular(name + enc.ext) {
			found = append(found, enc)
		}
	}
	return found
}

// serveCopy serves the compressed file name as coding with the original's contentType; false when it can't be opened.
func serveCopy(w http.ResponseWriter, r *http.Request, name, coding, contentType string) bool {
	f, err := os.Open(name)
	if err != nil {
		return false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return false
	}
	w.Header().Set("Content-Encoding", coding)
	w.Header().Set("Content-Type", contentType)
	http.ServeContent(w, r, name, info.ModTime(), f)
	return true
}

// withPrecompressed serves the first copy of encs that the file fileFor(path) has and the client
// accepts, and next otherwise. fileFor returns "" for a request that is not served from a file.
// A file with a copy gets "Vary: Accept-Encoding" either way, so caches keep both versions apart.
func withPrecompressed(encs []encoding, fileFor func(urlPath string) string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := fileFor(r.URL.Path)
		copies := compressedCopies(name, encs)
		if len(copies) == 0 {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Add("Vary", "Accept-Encoding")
		contentType := mime.TypeByExtension(filepath.Ext(name)) // unknown types would be sniffed from compressed bytes
		for _, enc := range copies {
			if contentType != "" && accepts(r.Header.Get("Accept-Encoding"), enc.coding) &&
				serveCopy(w, r, name+enc.ext, enc.coding, contentType) {
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// fileUnder maps a request path to the regular file under root, "" when there is none.
func fileUnder(root string) func(urlPath string) string {
	return func(urlPath string) string {
		if name := filePath(root, urlPath); isRegular(name) {
			return name
		}
		return ""
	}
}

// spaFile maps a request path to the file spa serves it from: the regular file under distDir,
// indexPath when nothing exists there, "" for a directory (left to http.FileServer).
func spaFile(distDir, indexPath string) func(urlPath string) string {
	return func(urlPath string) string {
		if !exists(distDir, urlPath) {
			return indexPath
		}
		return fileUnder(distDir)(urlPath)
	}
}

// spa serves the file at the request's path when distDir has one, and indexPath otherwise.
func spa(distDir, indexPath string) http.Handler {
	files := http.FileServer(http.Dir(distDir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if exists(distDir, r.URL.Path) {
			files.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, indexPath)
	})
}

// newHandler routes assets/ to the files under distDir and every other path to spa, with their
// Cache-Control values and pre-compressed copies of encs.
func newHandler(distDir, indexPath, indexCache, assetsCache string, encs []encoding) http.Handler {
	assets := withPrecompressed(encs, fileUnder(distDir), http.FileServer(http.Dir(distDir)))
	app := withPrecompressed(encs, spaFile(distDir, indexPath), spa(distDir, indexPath))
	mux := http.NewServeMux()
	mux.Handle("/assets/", withFileCacheControl(assetsCache, distDir, assets))
	mux.Handle("/", withCacheControl(indexCache, app))
	return mux
}

func main() {
	indexCache := flag.String("index-cache", "", `Cache-Control for index.html and the other files outside assets/, e.g. "no-cache" (default: none)`)
	assetsCache := flag.String("assets-cache", "", `Cache-Control for the files under assets/, e.g. "public, max-age=31536000, immutable" (default: none)`)
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "Usage: thumb [flags] <path/to/index.html> <port>")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() < 2 {
		flag.Usage()
		os.Exit(2)
	}

	indexAbs, err := filepath.Abs(flag.Arg(0))
	if err != nil {
		log.Fatalf("Invalid index.html path: %v", err)
	}
	distDir := filepath.Dir(indexAbs) // the folder index.html is in
	port := flag.Arg(1)

	encs := []encoding{{coding: "br", ext: ".br"}, {coding: "gzip", ext: ".gz"}} // best first

	addr := ":" + port
	log.Printf("Serving %s and assets/ from %s on http://localhost%s\n", indexAbs, distDir, addr)
	log.Fatal(http.ListenAndServe(addr, newHandler(distDir, indexAbs, *indexCache, *assetsCache, encs)))
}
