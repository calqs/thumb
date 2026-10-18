// thumb serves a single-page app: files under assets/ as they are, any other file that exists next
// to index.html, and index.html itself for every other path, so the app's router handles the
// address. Optional flags set Cache-Control headers: typically "no-cache" on the index, so a
// redeploy is picked up on the next load, and a long "immutable" on the hashed assets.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

// exists reports whether urlPath names a file under root.
func exists(root, urlPath string) bool {
	_, err := os.Stat(filepath.Join(root, filepath.Clean("/"+urlPath)))
	return err == nil
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

	mux := http.NewServeMux()
	mux.Handle("/assets/", withFileCacheControl(*assetsCache, distDir, http.FileServer(http.Dir(distDir))))
	mux.Handle("/", withCacheControl(*indexCache, spa(distDir, indexAbs)))

	addr := ":" + port
	log.Printf("Serving %s and assets/ from %s on http://localhost%s\n", indexAbs, distDir, addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
