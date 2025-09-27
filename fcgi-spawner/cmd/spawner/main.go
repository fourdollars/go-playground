package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	fcgiclient "github.com/tomasen/fcgi_client"
)

var (
	webRoot            string
	staticRoot         string
	socketDir          string
	listenAddr         string
	defaultIdleTimeout time.Duration // New global variable for default idle timeout
	staticFileServer   http.Handler
	childProcessesMu   sync.Mutex
	childProcesses     = make(map[string]*childProcess)
)

type childProcess struct {
	cmd           *exec.Cmd
	socketPath    string
	lastUsed      time.Time
	binaryPath    string
	idleTimeout   time.Duration // New field for idle timeout
	binaryModTime time.Time     // New field to store the modification time of the binary
}

func cleanupChildProcesses() {
	for {
		childProcessesMu.Lock()
		for appPath, child := range childProcesses {
			// Check if process is still alive. On Unix, signal 0 can be used to check for existence.
			// If the process is not alive, an error will be returned.
			if child.cmd.Process.Signal(syscall.Signal(0)) != nil {
				log.Printf("Child process for %s (PID: %d) is no longer running, removing from map.", appPath, child.cmd.Process.Pid)
				// Wait for the process to ensure it's reaped and doesn't become a zombie
				if _, err := child.cmd.Process.Wait(); err != nil {
					log.Printf("Error waiting for child process %d: %v", child.cmd.Process.Pid, err)
				}
				delete(childProcesses, appPath)
				if err := os.Remove(child.socketPath); err != nil {
					log.Printf("Error removing socket file %s: %v", child.socketPath, err)
				}
				continue // Move to the next child process
			}

			// Check for idle timeout
			if defaultIdleTimeout > 0 && time.Since(child.lastUsed) > defaultIdleTimeout {
				log.Printf("Child process for %s (PID: %d) has been idle for %s, terminating.", appPath, child.cmd.Process.Pid, time.Since(child.lastUsed).Round(time.Second))
				_ = child.cmd.Process.Kill() // Terminate the process
				// Wait for the process to ensure it's reaped and doesn't become a zombie
				if _, err := child.cmd.Process.Wait(); err != nil {
					log.Printf("Error waiting for child process %d: %v", child.cmd.Process.Pid, err)
				}
				delete(childProcesses, appPath)
				if err := os.Remove(child.socketPath); err != nil {
					log.Printf("Error removing socket file %s: %v", child.socketPath, err)
				}
			}
		}
		childProcessesMu.Unlock()
		time.Sleep(5 * time.Second) // Check every 5 seconds
	}
}

func main() {
	flag.StringVar(&webRoot, "webRoot", "/web", "Root directory for web files")
	flag.StringVar(&staticRoot, "staticRoot", "", "Optional root directory for static files. If specified, files in this directory will be served.")
	flag.StringVar(&socketDir, "socketDir", "/tmp/fcgi-sockets", "Directory for FastCGI application sockets")
	flag.StringVar(&listenAddr, "listenAddr", ":8080", "Address for the spawner to listen on (e.g., :8080)")
	flag.DurationVar(&defaultIdleTimeout, "idleTimeout", 5*time.Minute, "Idle timeout for child processes (e.g., 1m, 5m, 1h)")
	flag.Parse()

	if staticRoot != "" {
		info, err := os.Stat(staticRoot)
		if err != nil {
			log.Fatalf("Error accessing staticRoot %s: %v", staticRoot, err)
		}
		if !info.IsDir() {
			log.Fatalf("staticRoot %s is not a directory", staticRoot)
		}
		log.Printf("Enabling static file serving from %s", staticRoot)
		staticFileServer = http.FileServer(noHiddenFS{http.Dir(staticRoot)})
	}

	// The spawner is a regular HTTP server that will be started by supervisor.
	// Nginx will proxy requests to this server.

	// Start the cleanup goroutine
	go cleanupChildProcesses()

	// Start the file watcher goroutine
	go watchFcgiBinaries()

	http.HandleFunc("/", spawnerHandler)
	log.Printf("Spawner listening on %s", listenAddr)
	if err := http.ListenAndServe(listenAddr, nil); err != nil {
		log.Fatal(err)
	}
}

func watchFcgiBinaries() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal("Failed to create file watcher:", err)
	}
	defer watcher.Close()

	err = watcher.Add(webRoot)
	if err != nil {
		log.Fatal("Failed to add webRoot to watcher:", err)
	}

	log.Printf("Watching directory %s for changes to FCGI binaries", webRoot)

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Write == fsnotify.Write {
				if strings.HasSuffix(event.Name, ".fcgi") {
					appPath := event.Name
					log.Printf("FCGI binary changed: %s. Terminating existing child process if any.", appPath)

					childProcessesMu.Lock()
					if child, exists := childProcesses[appPath]; exists {
						log.Printf("Terminating old child process for %s (PID: %d)", appPath, child.cmd.Process.Pid)
						_ = child.cmd.Process.Kill()
						_ = os.Remove(child.socketPath) // Clean up socket file
						delete(childProcesses, appPath)
					}
					childProcessesMu.Unlock()
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Println("Watcher error:", err)
		}
	}
}

// noHiddenFS is a file system that hides dot files.
type noHiddenFS struct {
	fs http.FileSystem
}

// Open implements the http.FileSystem interface.
func (nhfs noHiddenFS) Open(name string) (http.File, error) {
	// Disallow browsing of hidden files/directories
	if strings.Contains(name, "/.") {
		return nil, os.ErrNotExist
	}

	file, err := nhfs.fs.Open(name)
	if err != nil {
		return nil, err
	}
	return noHiddenFile{file}, nil
}

// noHiddenFile is a file that filters out hidden files from directory listings.
type noHiddenFile struct {
	http.File
}

// Readdir implements the http.File interface and filters out hidden files.
func (nhf noHiddenFile) Readdir(count int) ([]os.FileInfo, error) {
	files, err := nhf.File.Readdir(count)
	if err != nil {
		return nil, err
	}

	var visibleFiles []os.FileInfo
	for _, f := range files {
		if !strings.HasPrefix(f.Name(), ".") {
			visibleFiles = append(visibleFiles, f)
		}
	}
	return visibleFiles, nil
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

	// Check if the requested path is an executable FCGI application
	fileInfo, err := os.Stat(targetPath)
	if err == nil && fileInfo.Mode().IsRegular() && (fileInfo.Mode().Perm()&0111 != 0) && strings.HasSuffix(targetPath, ".fcgi") {
		child, err := getOrCreateChild(targetPath)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			log.Printf("Error getting or creating child process for %s: %v", targetPath, err)
			return
		}
		proxyRequest(w, r, child)
		return
	}

	// If not an FCGI app, try serving as a static file
	if staticFileServer != nil {
		staticFileServer.ServeHTTP(w, r)
		return
	}

	// If we reach here, it's a 404
	http.Error(w, "404 Not Found", http.StatusNotFound)
	log.Printf("Requested path %s is not a valid FCGI application and static file serving is disabled.", r.URL.Path)
}

func getOrCreateChild(appPath string) (*childProcess, error) {
	childProcessesMu.Lock()
	defer childProcessesMu.Unlock()

	fileInfo, err := os.Stat(appPath)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("application not found: %s", appPath)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get file info for %s: %v", appPath, err)
	}
	currentModTime := fileInfo.ModTime()

	if child, exists := childProcesses[appPath]; exists {
		// Check if process is still alive and binary hasn't changed
		if (child.cmd.ProcessState == nil || !child.cmd.ProcessState.Exited()) && !currentModTime.After(child.binaryModTime) {
			child.lastUsed = time.Now()
			return child, nil
		}
		// Process has exited or binary has changed, so we'll terminate the old one and create a new one.
		log.Printf("Child process for %s (PID: %d) has exited or binary changed. Terminating old process and restarting...", appPath, child.cmd.Process.Pid)
		// Attempt graceful shutdown first
		if child.cmd.Process != nil {
			if err := child.cmd.Process.Signal(syscall.SIGTERM); err != nil {
				log.Printf("Error sending SIGTERM to child process %d: %v", child.cmd.Process.Pid, err)
			}
			// Give it a moment to shut down gracefully
			time.Sleep(1 * time.Second)

			// If it's still alive, forcefully kill it
			if child.cmd.Process.Signal(syscall.Signal(0)) == nil { // Check if process is still alive
				if err := child.cmd.Process.Kill(); err != nil {
					log.Printf("Error sending SIGKILL to child process %d: %v", child.cmd.Process.Pid, err)
				}
			}
		}
		// Wait for the process to ensure it's reaped and doesn't become a zombie
		if _, err := child.cmd.Process.Wait(); err != nil {
			log.Printf("Error waiting for child process %d: %v", child.cmd.Process.Pid, err)
		}
		if err := os.Remove(child.socketPath); err != nil {
			log.Printf("Error removing socket file %s: %v", child.socketPath, err)
		}
		delete(childProcesses, appPath)
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
		cmd:           cmd,
		socketPath:    socketPath,
		lastUsed:      time.Now(),
		binaryPath:    appPath,
		idleTimeout:   defaultIdleTimeout, // Initialize with the default idle timeout
		binaryModTime: currentModTime,     // Store the current modification time
	}
	childProcesses[appPath] = child

	log.Printf("Started new child process for %s (PID: %d) on socket %s", appPath, cmd.Process.Pid, child.socketPath)

	return child, nil
}

func proxyRequest(w http.ResponseWriter, r *http.Request, child *childProcess) {
	childProcessesMu.Lock()
	child.lastUsed = time.Now()
	childProcessesMu.Unlock()

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
