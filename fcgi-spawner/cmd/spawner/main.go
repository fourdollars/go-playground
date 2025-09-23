package main

import (
	"log"
	"net"
	"net/http"
	"net/http/cgi"
	"net/http/fcgi"
	"os"
	"path/filepath"
	"strings"
)

const (
	webRoot = "/var/www/html"
)

func main() {
	// systemd provides the socket listener as file descriptor 3
	f := os.NewFile(3, "socket")
	if f == nil {
		log.Fatal("Spawner must be run from systemd socket activation.")
	}
	l, err := net.FileListener(f)
	if err != nil {
		log.Fatalf("Failed to create listener from file: %v", err)
	}
	defer l.Close()

	handler := http.HandlerFunc(spawnerHandler)
	if err := fcgi.Serve(l, handler); err != nil {
		log.Printf("fcgi.Serve failed: %v", err)
	}
}

func spawnerHandler(w http.ResponseWriter, r *http.Request) {
	scriptFilename := r.Header.Get("SCRIPT_FILENAME")
	if scriptFilename == "" {
		// Nginx's fastcgi_param SCRIPT_FILENAME is what we use to find the app.
		// It's passed as a header by the fcgi package.
		http.Error(w, "Internal Server Error: SCRIPT_FILENAME not set", http.StatusInternalServerError)
		log.Println("SCRIPT_FILENAME not set in FastCGI request")
		return
	}

	// Security: Use Base to prevent directory traversal. e.g. /app-hello.fcgi
	appName := filepath.Base(scriptFilename)
	targetPath := filepath.Join(webRoot, appName)

	// Security: Double-check the path is within our web root.
	if !strings.HasPrefix(targetPath, webRoot) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		log.Printf("Forbidden: Attempted directory traversal: %s", scriptFilename)
		return
	}

	// Check if the file exists and is executable
	info, err := os.Stat(targetPath)
	if os.IsNotExist(err) {
		http.NotFound(w, r)
		log.Printf("Not Found: %s", targetPath)
		return
	}
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		log.Printf("Error stating file %s: %v", targetPath, err)
		return
	}
	if info.Mode()&0111 == 0 { // Check for execute permission
		http.Error(w, "Forbidden: File is not executable", http.StatusForbidden)
		log.Printf("Forbidden: %s is not executable", targetPath)
		return
	}

	// Use cgi.Handler to execute the script.
	// The child process will be a Go app using fcgi.Serve(nil, ...), which
	// means it acts like a CGI script reading from stdin and writing to stdout.
	cgiHandler := &cgi.Handler{
		Path:   targetPath,
		Root:   "/", // Root is not super relevant here as Path is absolute
		Dir:    webRoot,
		Stderr: log.Writer(), // Log child process errors
	}

	cgiHandler.ServeHTTP(w, r)
}
