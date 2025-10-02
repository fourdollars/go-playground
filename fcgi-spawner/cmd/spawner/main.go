package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
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

// Config holds the spawner's configuration.
type Config struct {
	WebRoot            string
	StaticRoot         string
	SocketDir          string
	ListenAddr         string
	DefaultIdleTimeout time.Duration
}

// loadConfig parses command-line flags and returns a Config struct.
func loadConfig() *Config {
	cfg := &Config{}
	flag.StringVar(&cfg.WebRoot, "webRoot", "/web", "Root directory for web files")
	flag.StringVar(&cfg.StaticRoot, "staticRoot", "", "Optional root directory for static files. If specified, files in this directory will be served.")
	flag.StringVar(&cfg.SocketDir, "socketDir", "", "Directory for FastCGI application sockets. If empty, stdio mode is used.")
	flag.StringVar(&cfg.ListenAddr, "listenAddr", ":8080", "Address for the spawner to listen on (e.g., :8080)")
	flag.DurationVar(&cfg.DefaultIdleTimeout, "idleTimeout", 5*time.Minute, "Idle timeout for child processes (e.g., 1m, 5m, 1h)")
	flag.Parse()
	return cfg
}

// Spawner manages FastCGI applications and serves static files.
type Spawner struct {
	Config           *Config
	staticFileServer http.Handler
	childProcessesMu sync.Mutex
	childProcesses   map[string]*childProcess
}

// NewSpawner creates and initializes a new Spawner instance.
func NewSpawner(cfg *Config) *Spawner {
	s := &Spawner{
		Config:         cfg,
		childProcesses: make(map[string]*childProcess),
	}

	if cfg.StaticRoot != "" {
		info, err := os.Stat(cfg.StaticRoot)
		if err != nil {
			log.Fatalf("Error accessing staticRoot %s: %v", cfg.StaticRoot, err)
		}
		if !info.IsDir() {
			log.Fatalf("staticRoot %s is not a directory", cfg.StaticRoot)
		}
		log.Printf("Enabling static file serving from %s", cfg.StaticRoot)
		s.staticFileServer = http.FileServer(noHiddenFS{http.Dir(cfg.StaticRoot)})
	}
	return s
}

type processInterface interface {
	Signal(os.Signal) error
	Wait() (*os.ProcessState, error)
	Kill() error
	Pid() int // Add Pid() for logging
}

type cmdInterface interface {
	Start() error
	Process() processInterface
	ProcessState() *os.ProcessState
	Path() string // Add Path() for SCRIPT_FILENAME
}

type childProcess struct {
	cmd           cmdInterface
	socketPath    string
	lastUsed      time.Time
	binaryPath    string
	idleTimeout   time.Duration
	binaryModTime time.Time
	listener      net.Listener // Add listener for stdio apps
}

// execCmdWrapper implements cmdInterface for *exec.Cmd
type execCmdWrapper struct {
	cmd *exec.Cmd
}

func (w *execCmdWrapper) Start() error {
	return w.cmd.Start()
}

func (w *execCmdWrapper) Process() processInterface {
	if w.cmd.Process == nil {
		return nil
	}
	return &osProcessWrapper{w.cmd.Process}
}

func (w *execCmdWrapper) ProcessState() *os.ProcessState {
	return w.cmd.ProcessState
}

func (w *execCmdWrapper) Path() string {
	return w.cmd.Path
}

// osProcessWrapper implements processInterface for *os.Process
type osProcessWrapper struct {
	process *os.Process
}

func (w *osProcessWrapper) Signal(sig os.Signal) error {
	return w.process.Signal(sig)
}

func (w *osProcessWrapper) Wait() (*os.ProcessState, error) {
	return w.process.Wait()
}

func (w *osProcessWrapper) Kill() error {
	return w.process.Kill()
}

func (w *osProcessWrapper) Pid() int {
	return w.process.Pid
}

func (s *Spawner) cleanupChildProcesses() {
	for {
		s.childProcessesMu.Lock()
		for appPath, child := range s.childProcesses {
			// Check if process is still alive. On Unix, signal 0 can be used to check for existence.
			// If the process is not alive, an error will be returned.
			if child.cmd.Process() != nil && child.cmd.Process().Signal(syscall.Signal(0)) != nil {
				log.Printf("Child process for %s (PID: %d) is no longer running, removing from map.", appPath, child.cmd.Process().Pid())
				// Wait for the process to ensure it's reaped and doesn't become a zombie
				if _, err := child.cmd.Process().Wait(); err != nil {
					log.Printf("Error waiting for child process %d: %v", child.cmd.Process().Pid(), err)
				}
				if child.listener != nil {
					child.listener.Close()
				} else {
					if err := os.Remove(child.socketPath); err != nil && !os.IsNotExist(err) {
						log.Printf("Error removing socket file %s: %v", child.socketPath, err)
					}
				}
				delete(s.childProcesses, appPath)
				continue // Move to the next child process
			}

			// Check for idle timeout
			if s.Config.DefaultIdleTimeout > 0 && time.Since(child.lastUsed) > s.Config.DefaultIdleTimeout {
				log.Printf("Child process for %s (PID: %d) has been idle for %s, terminating.", appPath, child.cmd.Process().Pid(), time.Since(child.lastUsed).Round(time.Second))
				_ = child.cmd.Process().Kill() // Terminate the process
				// Wait for the process to ensure it's reaped and doesn't become a zombie
				if _, err := child.cmd.Process().Wait(); err != nil {
					log.Printf("Error waiting for child process %d: %v", child.cmd.Process().Pid(), err)
				}
				if child.listener != nil {
					child.listener.Close()
				} else {
					if err := os.Remove(child.socketPath); err != nil && !os.IsNotExist(err) {
						log.Printf("Error removing socket file %s: %v", child.socketPath, err)
					}
				}
				delete(s.childProcesses, appPath)
			}
		}
		s.childProcessesMu.Unlock()
		time.Sleep(5 * time.Second) // Check every 5 seconds
	}
}

// logStream reads from a stream (stdout/stderr) and logs each line with a prefix.
func logStream(stream io.ReadCloser, appPath string, pid int, streamName string) {
	scanner := bufio.NewScanner(stream)
	for scanner.Scan() {
		log.Printf("[%s/%d %s] %s", filepath.Base(appPath), pid, streamName, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		log.Printf("Error reading from %s stream for %s (PID: %d): %v", streamName, appPath, pid, err)
	}
}

func main() {
	cfg := loadConfig() // Load configuration
	spawner := NewSpawner(cfg)

	// The spawner is a regular HTTP server that will be started by supervisor.
	// Nginx will proxy requests to this server.

	// Start the cleanup goroutine
	go spawner.cleanupChildProcesses()

	// Start the file watcher goroutine
	go spawner.watchFcgiBinaries()

	http.HandleFunc("/", spawner.spawnerHandler)
	log.Printf("Spawner listening on %s", spawner.Config.ListenAddr)
	if err := http.ListenAndServe(spawner.Config.ListenAddr, nil); err != nil {
		log.Fatal(err)
	}
}

func (s *Spawner) watchFcgiBinaries() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal("Failed to create file watcher:", err)
	}
	defer watcher.Close()

	err = watcher.Add(s.Config.WebRoot)
	if err != nil {
		log.Fatal("Failed to add webRoot to watcher:", err)
	}

	log.Printf("Watching directory %s for changes to FCGI binaries", s.Config.WebRoot)

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

					s.childProcessesMu.Lock()
					if child, exists := s.childProcesses[appPath]; exists {
						log.Printf("Terminating old child process for %s (PID: %d)", appPath, child.cmd.Process().Pid())
						_ = child.cmd.Process().Kill()
						_ = os.Remove(child.socketPath) // Clean up socket file
						delete(s.childProcesses, appPath)
					}
					s.childProcessesMu.Unlock()
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

func (s *Spawner) spawnerHandler(w http.ResponseWriter, r *http.Request) {
	scriptPath := r.URL.Path
	if scriptPath == "" {
		http.Error(w, "Internal Server Error: script path is empty", http.StatusInternalServerError)
		log.Println("script path is empty in request")
		return
	}

	appName := filepath.Base(scriptPath)
	targetPath := filepath.Join(s.Config.WebRoot, appName)

	if !strings.HasPrefix(targetPath, s.Config.WebRoot) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		log.Printf("Forbidden: Attempted directory traversal: %s", scriptPath)
		return
	}

	// Check if the requested path is an executable FCGI application
	fileInfo, err := os.Stat(targetPath)
	if err == nil && fileInfo.Mode().IsRegular() && (fileInfo.Mode().Perm()&0111 != 0) && strings.HasSuffix(targetPath, ".fcgi") {
		child, err := s.getOrCreateChild(targetPath)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			log.Printf("Error getting or creating child process for %s: %v", targetPath, err)
			return
		}
		s.proxyRequest(w, r, child)
		return
	}

	// If not an FCGI app, try serving as a static file
	if s.staticFileServer != nil {
		s.staticFileServer.ServeHTTP(w, r)
		return
	}

	// If we reach here, it's a 404
	http.Error(w, "404 Not Found", http.StatusNotFound)
	log.Printf("Requested path %s is not a valid FCGI application and static file serving is disabled.", r.URL.Path)
}

func (s *Spawner) getOrCreateChild(appPath string) (*childProcess, error) {
	s.childProcessesMu.Lock()
	defer s.childProcessesMu.Unlock()

	fileInfo, err := os.Stat(appPath)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("application not found: %s", appPath)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get file info for %s: %v", appPath, err)
	}
	currentModTime := fileInfo.ModTime()

	if child, exists := s.childProcesses[appPath]; exists {
		// Check if process is still alive and binary hasn't changed
		if (child.cmd.ProcessState() == nil || !child.cmd.ProcessState().Exited()) && !currentModTime.After(child.binaryModTime) {
			child.lastUsed = time.Now()
			return child, nil
		}
		// Process has exited or binary has changed, so we'll terminate the old one and create a new one.
		log.Printf("Child process for %s (PID: %d) has exited or binary changed. Terminating old process and restarting...", appPath, child.cmd.Process().Pid())
		// Attempt graceful shutdown first
		if child.cmd.Process() != nil {
			if err := child.cmd.Process().Signal(syscall.SIGTERM); err != nil {
				log.Printf("Error sending SIGTERM to child process %d: %v", child.cmd.Process().Pid(), err)
			}
			// Give it a moment to shut down gracefully
			time.Sleep(1 * time.Second)

			// If it's still alive, forcefully kill it
			if child.cmd.Process() != nil && child.cmd.Process().Signal(syscall.Signal(0)) == nil { // Check if process is still alive
				if err := child.cmd.Process().Kill(); err != nil {
					log.Printf("Error sending SIGKILL to child process %d: %v", child.cmd.Process().Pid(), err)
				}
			}
		}
		// Wait for the process to ensure it's reaped and doesn't become a zombie
		if _, err := child.cmd.Process().Wait(); err != nil {
			log.Printf("Error waiting for child process %d: %v", child.cmd.Process().Pid(), err)
		}
		if child.listener != nil {
			child.listener.Close()
		} else {
			if err := os.Remove(child.socketPath); err != nil && !os.IsNotExist(err) {
				log.Printf("Error removing socket file %s: %v", child.socketPath, err)
			}
		}
		delete(s.childProcesses, appPath)
	}

	// Load environment variables from .env file if it exists
	var childEnv []string
	envFilePath := strings.TrimSuffix(appPath, ".fcgi") + ".env"
	if _, err := os.Stat(envFilePath); err == nil {
		log.Printf("Loading environment file: %s", envFilePath)
		envFile, err := os.Open(envFilePath)
		if err != nil {
			return nil, fmt.Errorf("could not open env file %s: %v", envFilePath, err)
		}
		defer envFile.Close()

		childEnv = os.Environ() // Start with parent's environment
		scanner := bufio.NewScanner(envFile)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" && !strings.HasPrefix(line, "#") {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					childEnv = append(childEnv, line)
				}
			}
		}
	}

	useSocketMode := s.Config.SocketDir != ""
	var socketPath string
	if useSocketMode {
		socketPath = filepath.Join(s.Config.SocketDir, filepath.Base(appPath)+".sock")
		if err := os.MkdirAll(s.Config.SocketDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create socket directory: %v", err)
		}
		// Clean up old socket file if it exists
		_ = os.Remove(socketPath)
	} else {
		// Use an abstract socket for stdio mode
		socketPath = filepath.Join("/tmp/fcgi-spawner-sockets", filepath.Base(appPath)+".sock")
		socketPath = "\x00" + socketPath
	}

	var cmd *exec.Cmd
	var ln net.Listener

	if useSocketMode {
		cmd = exec.Command(appPath, socketPath)
	} else {
		cmd = exec.Command(appPath)
		var err error
		ln, err = net.Listen("unix", socketPath)
		if err != nil {
			return nil, fmt.Errorf("failed to create listener for stdio app: %v", err)
		}

		unixListener, ok := ln.(*net.UnixListener)
		if !ok {
			ln.Close()
			return nil, fmt.Errorf("listener was not a UnixListener")
		}

		listenerFile, err := unixListener.File()
		if err != nil {
			ln.Close()
			return nil, fmt.Errorf("failed to get listener file for child: %v", err)
		}
		defer listenerFile.Close() // We can close the file descriptor copy after start
		cmd.Stdin = listenerFile
	}

	if childEnv != nil {
		cmd.Env = childEnv
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		if ln != nil {
			ln.Close()
		}
		return nil, fmt.Errorf("failed to create stderr pipe for %s: %v", appPath, err)
	}

	var stdoutToLog io.ReadCloser
	if useSocketMode { // Only capture stdout for socket mode
		var err error
		stdoutToLog, err = cmd.StdoutPipe()
		if err != nil {
			if ln != nil {
				ln.Close()
			}
			return nil, fmt.Errorf("failed to create stdout pipe for %s: %v", appPath, err)
		}
	}

	if err := cmd.Start(); err != nil {
		if ln != nil {
			ln.Close()
		}
		return nil, fmt.Errorf("failed to start application %s: %v", appPath, err)
	}

	go logStream(stderr, appPath, cmd.Process.Pid, "stderr")
	if stdoutToLog != nil {
		go logStream(stdoutToLog, appPath, cmd.Process.Pid, "stdout")
	}

	// Wait for the child to be ready by dialing the socket.
	var conn net.Conn
	var dialErr error
	for i := 0; i < 100; i++ {
		conn, dialErr = net.DialTimeout("unix", socketPath, 50*time.Millisecond)
		if dialErr == nil {
			conn.Close()
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if dialErr != nil {
		log.Printf("Failed to connect to child socket %s after timeout: %v", socketPath, dialErr)
		// Attempt to kill the process we just started, as it's not responding
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		if ln != nil {
			ln.Close()
		}
		return nil, fmt.Errorf("failed to connect to child socket: %v", dialErr)
	}

	child := &childProcess{
		cmd:           &execCmdWrapper{cmd: cmd},
		socketPath:    socketPath,
		lastUsed:      time.Now(),
		binaryPath:    appPath,
		idleTimeout:   s.Config.DefaultIdleTimeout,
		binaryModTime: currentModTime,
		listener:      ln, // Store the listener
	}
	s.childProcesses[appPath] = child

	if useSocketMode {
		log.Printf("Started new socket child process for %s (PID: %d) on socket %s", appPath, child.cmd.Process().Pid(), child.socketPath)
	} else {
		log.Printf("Started new stdio child process for %s (PID: %d)", appPath, child.cmd.Process().Pid())
	}

	return child, nil
}

func (s *Spawner) proxyRequest(w http.ResponseWriter, r *http.Request, child *childProcess) {
	s.childProcessesMu.Lock()
	child.lastUsed = time.Now()
	s.childProcessesMu.Unlock()

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
	env["SCRIPT_FILENAME"] = child.cmd.Path()
	env["SCRIPT_NAME"] = r.URL.Path
	env["REQUEST_URI"] = r.URL.RequestURI()
	env["DOCUMENT_URI"] = r.URL.Path
	env["DOCUMENT_ROOT"] = s.Config.WebRoot
	env["SERVER_SOFTWARE"] = "go-fcgi-spawner"
	env["REMOTE_ADDR"] = r.RemoteAddr
	env["HTTP_HOST"] = r.Host

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
