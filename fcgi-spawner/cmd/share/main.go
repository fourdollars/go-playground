package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/fcgi"
	"os"
	"path/filepath"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <socket-path>\n", os.Args[0])
		os.Exit(1)
	}
	socketPath := os.Args[1]

	l, err := net.Listen("unix", socketPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "net.Listen failed: %v\n", err)
		os.Exit(1)
	}
	defer l.Close()

	handler := &ShareHandler{
		Workspace: os.Getenv("WORKSPACE"),
		Salt:      os.Getenv("SALT"),
		Password:  os.Getenv("PASSWORD"),
	}

	if handler.Workspace == "" {
		fmt.Fprintln(os.Stderr, "Warning: WORKSPACE env not set, using /tmp")
		handler.Workspace = "/tmp"
	}
	if handler.Salt == "" {
		fmt.Fprintln(os.Stderr, "Warning: SALT env not set")
	}
	if handler.Password == "" {
		fmt.Fprintln(os.Stderr, "Warning: PASSWORD env not set - uploads will be unprotected")
	}

	if err := os.MkdirAll(handler.Workspace, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create workspace: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Starting share.fcgi on %s\n", socketPath)
	if err := fcgi.Serve(l, handler); err != nil {
		fmt.Fprintf(os.Stderr, "fcgi.Serve failed: %v\n", err)
		os.Exit(1)
	}
}

type ShareHandler struct {
	Workspace string
	Salt      string
	Password  string
}

func (h *ShareHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		h.handleUpload(w, r)
	} else if r.Method == http.MethodGet {
		h.handleDownload(w, r)
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *ShareHandler) computeSecret(content io.Reader) (string, error) {
	hasher := sha256.New()
	hasher.Write([]byte(h.Salt))
	if _, err := io.Copy(hasher, content); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func (h *ShareHandler) handleUpload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "Failed to parse form: "+err.Error(), http.StatusBadRequest)
		return
	}

	if h.Password != "" {
		providedPassword := r.FormValue("password")
		if providedPassword != h.Password {
			http.Error(w, "Unauthorized: Invalid password", http.StatusUnauthorized)
			return
		}
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "No file provided (key 'file' missing)", http.StatusBadRequest)
		return
	}
	defer file.Close()

	filename := filepath.Base(header.Filename)
	if filename == "" || filename == "." {
		http.Error(w, "Invalid filename", http.StatusBadRequest)
		return
	}

	savePath := filepath.Join(h.Workspace, filename)
	outFile, err := os.Create(savePath)
	if err != nil {
		http.Error(w, "Failed to create file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	success := false
	defer func() {
		if !success {
			outFile.Close()
			os.Remove(savePath)
		}
	}()

	hasher := sha256.New()
	hasher.Write([]byte(h.Salt))

	writer := io.MultiWriter(outFile, hasher)

	if _, err := io.Copy(writer, file); err != nil {
		http.Error(w, "Failed to save file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := outFile.Close(); err != nil {
		http.Error(w, "Failed to close file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	secret := hex.EncodeToString(hasher.Sum(nil))
	success = true

	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}

	host := r.Host
	if host == "" {
		host = "localhost"
	}

	shareURL := fmt.Sprintf("%s://%s%s?file=%s&secret=%s", scheme, host, r.URL.Path, filename, secret)

	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(shareURL))
}

func (h *ShareHandler) handleDownload(w http.ResponseWriter, r *http.Request) {
	filename := r.URL.Query().Get("file")
	providedSecret := r.URL.Query().Get("secret")

	if filename == "" || providedSecret == "" {
		http.Error(w, "Missing file or secret param", http.StatusBadRequest)
		return
	}

	cleanFilename := filepath.Base(filename)
	filePath := filepath.Join(h.Workspace, cleanFilename)

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	f, err := os.Open(filePath)
	if err != nil {
		http.Error(w, "Failed to open file", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	hasher := sha256.New()
	hasher.Write([]byte(h.Salt))
	if _, err := io.Copy(hasher, f); err != nil {
		http.Error(w, "Failed to read file", http.StatusInternalServerError)
		return
	}
	computedSecret := hex.EncodeToString(hasher.Sum(nil))

	if providedSecret != computedSecret {
		http.Error(w, "Invalid secret", http.StatusForbidden)
		return
	}

	if _, err := f.Seek(0, 0); err != nil {
		http.Error(w, "Failed to rewind file", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", cleanFilename))
	w.Header().Set("Content-Type", "application/octet-stream")

	if _, err := io.Copy(w, f); err != nil {
		fmt.Fprintf(os.Stderr, "Error sending file: %v\n", err)
		return
	}

	f.Close()
	if err := os.Remove(filePath); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to delete file %s: %v\n", filePath, err)
	}
}
