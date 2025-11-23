package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
)

func main() {
	if len(os.Args) < 3 {
		log.Fatal("Usage: ./server <path/to/index.html> <port>")
	}

	indexPath := os.Args[1]
	port := os.Args[2]

	indexAbs, err := filepath.Abs(indexPath)
	if err != nil {
		log.Fatalf("Invalid index.html path: %v", err)
	}

	// Directory where index.html is located
	distDir := filepath.Dir(indexAbs)
	fs := http.FileServer(http.Dir(distDir))

	// Serve static files relative to distDir
	http.Handle("/assets/", fs) // correct: this serves distDir/assets/*

	// Always serve index.html for any other route
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, indexAbs)
	})

	addr := ":" + port
	log.Printf("Serving %s and assets/ from %s on http://localhost%s\n", indexAbs, distDir, addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
