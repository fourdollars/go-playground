package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	fcgiclient "github.com/tomasen/fcgi_client"
)

const (
	webRoot   = "/var/www/html"
	socketDir = "/tmp/fcgi-sockets"
)

type childProcess struct {
	cmd        *exec.Cmd
	socketPath string
	lastUsed   time.Time
}

var (
	childProcesses   = make(map[string]*childProcess)
	childProcessesMu sync.Mutex
)

func main() {
	// The spawner is a regular HTTP server that will be started by supervisor.
	// Nginx will proxy requests to this server.
	http.HandleFunc("/", spawnerHandler)
	log.Println("Spawner listening on :9000")
	if err := http.ListenAndServe(":9000", nil); err != nil {
		log.Fatal(err)
	}
}

func spawnerHandler(w http.ResponseWriter, r *http.Request) {
	scriptPath := r.URL.Path
	if scriptPath == "" {
		http.Error(w, "Internal Server Error: script path is empty", http.StatusInternalServerError)
		log.Println("script path is empty in request")
		return
	}

	appName := filepath.Base(scriptPath)
	targetPath := filepath.Join(webRoot, appName)

	if !strings.HasPrefix(targetPath, webRoot) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		log.Printf("Forbidden: Attempted directory traversal: %s", scriptPath)
		return
	}

	child, err := getOrCreateChild(targetPath)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		log.Printf("Error getting or creating child process for %s: %v", targetPath, err)
		return
	}

	proxyRequest(w, r, child)
}

func getOrCreateChild(appPath string) (*childProcess, error) {
	childProcessesMu.Lock()
	defer childProcessesMu.Unlock()

	if child, exists := childProcesses[appPath]; exists {
		// Simple keep-alive: just check if the process is still running.
		if child.cmd.ProcessState == nil || !child.cmd.ProcessState.Exited() {
			child.lastUsed = time.Now()
			return child, nil
		}
		// Process has exited, so we'll create a new one.
		log.Printf("Child process for %s has exited. Restarting...", appPath)
	}

	if _, err := os.Stat(appPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("application not found: %s", appPath)
	}

	socketPath := filepath.Join(socketDir, filepath.Base(appPath)+".sock")
	if err := os.MkdirAll(socketDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create socket directory: %v", err)
	}
	// Clean up old socket file if it exists
	_ = os.Remove(socketPath)

	cmd := exec.Command(appPath, socketPath)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start application %s: %v", appPath, err)
	}

	// Wait for the socket file to be created by the child
	for i := 0; i < 100; i++ {
		if _, err := os.Stat(socketPath); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	child := &childProcess{
		cmd:        cmd,
		socketPath: socketPath,
		lastUsed:   time.Now(),
	}
	childProcesses[appPath] = child

	log.Printf("Started new child process for %s (PID: %d) on socket %s", appPath, cmd.Process.Pid, child.socketPath)

	return child, nil
}

func proxyRequest(w http.ResponseWriter, r *http.Request, child *childProcess) {
	fcgi, err := fcgiclient.Dial("unix", child.socketPath)
	if err != nil {
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		log.Printf("Failed to connect to child application %s: %v", child.socketPath, err)
		return
	}
	defer fcgi.Close()

	env := make(map[string]string)
	env["REQUEST_METHOD"] = r.Method
	env["SERVER_PROTOCOL"] = r.Proto
	env["QUERY_STRING"] = r.URL.RawQuery
	env["CONTENT_TYPE"] = r.Header.Get("Content-Type")
	env["CONTENT_LENGTH"] = fmt.Sprintf("%d", r.ContentLength)
	env["SCRIPT_FILENAME"] = child.cmd.Path
	env["SCRIPT_NAME"] = r.URL.Path
	env["REQUEST_URI"] = r.URL.RequestURI()
	env["DOCUMENT_URI"] = r.URL.Path
	env["DOCUMENT_ROOT"] = webRoot
	env["SERVER_SOFTWARE"] = "go-fcgi-spawner"
	env["REMOTE_ADDR"] = r.RemoteAddr

	for name, headers := range r.Header {
		for _, h := range headers {
			env["HTTP_"+strings.ToUpper(strings.Replace(name, "-", "_", -1))] = h
		}
	}

	resp, err := fcgi.Request(env, r.Body)
	if err != nil {
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		log.Printf("FastCGI request failed: %v", err)
		return
	}

	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		log.Printf("Failed to copy response body: %v", err)
	}
}
