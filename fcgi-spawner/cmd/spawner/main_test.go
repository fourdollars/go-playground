package main

import (
	"flag"
	"os"
	"syscall"
	"testing"
	"time"
)

// Mocking infrastructure for os functions
var osRemove = os.Remove
var osStat = os.Stat

// Mock for os.Process
type mockProcess struct {
	pid    int
	signalErr error
	waitErr   error
	exited bool
}

func (m *mockProcess) Signal(sig os.Signal) error {
	return m.signalErr
}

func (m *mockProcess) Wait() (*os.ProcessState, error) {
	if m.waitErr != nil {
		return nil, m.waitErr
	}
	// Simulate a process state
	return &os.ProcessState{}, nil // Simplified for now
}

func (m *mockProcess) Kill() error {
	return nil // Simplified
}

func (m *mockProcess) Pid() int {
	return m.pid
}

// Mock for exec.Cmd
type mockCmd struct {
	process *mockProcess
	startErr error
	path string
}

func (m *mockCmd) Start() error {
	return m.startErr
}

func (m *mockCmd) Process() processInterface {
	if m.process == nil {
		return nil
	}
	return m.process
}

func (m *mockCmd) ProcessState() *os.ProcessState {
	if m.process == nil || !m.process.exited {
		return nil
	}
	return &os.ProcessState{} // Simplified
}

func (m *mockCmd) Path() string {
	return m.path
}

// Helper to reset mocks
func resetMocks() {
	osRemove = os.Remove
	osStat = os.Stat
}

// Helper function to reset flags for each test
func resetFlags() {
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
}

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want *Config
	}{
		{
			name: "default values",
			args: []string{},
			want: &Config{
				WebRoot:            "/web",
				StaticRoot:         "",
				SocketDir:          "/tmp/fcgi-sockets",
				ListenAddr:         ":8080",
				DefaultIdleTimeout: 5 * time.Minute,
			},
		},
		{
			name: "custom values",
			args: []string{
				"-webRoot", "/custom/web",
				"-staticRoot", "/custom/static",
				"-socketDir", "/custom/sockets",
				"-listenAddr", ":9000",
				"-idleTimeout", "10m",
			},
			want: &Config{
				WebRoot:            "/custom/web",
				StaticRoot:         "/custom/static",
				SocketDir:          "/custom/sockets",
				ListenAddr:         ":9000",
				DefaultIdleTimeout: 10 * time.Minute,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetFlags() // Reset flags before each test

			// Simulate command line arguments
			os.Args = append([]string{"test"}, tt.args...)

			got := loadConfig()

			// Compare Config fields
			if got.WebRoot != tt.want.WebRoot {
				t.Errorf("loadConfig() WebRoot = %v, want %v", got.WebRoot, tt.want.WebRoot)
			}
			if got.StaticRoot != tt.want.StaticRoot {
				t.Errorf("loadConfig() StaticRoot = %v, want %v", got.StaticRoot, tt.want.StaticRoot)
			}
			if got.SocketDir != tt.want.SocketDir {
				t.Errorf("loadConfig() SocketDir = %v, want %v", got.SocketDir, tt.want.SocketDir)
			}
			if got.ListenAddr != tt.want.ListenAddr {
				t.Errorf("loadConfig() ListenAddr = %v, want %v", got.ListenAddr, tt.want.ListenAddr)
			}
			if got.DefaultIdleTimeout != tt.want.DefaultIdleTimeout {
				t.Errorf("loadConfig() DefaultIdleTimeout = %v, want %v", got.DefaultIdleTimeout, tt.want.DefaultIdleTimeout)
			}
		})
	}
}

func TestNewSpawner(t *testing.T) {
	// Create a temporary directory for static files
	tempStaticDir, err := os.MkdirTemp("", "static-test")
	if err != nil {
		t.Fatalf("Failed to create temp static dir: %v", err)
	}
	defer os.RemoveAll(tempStaticDir)

	tests := []struct {
		name       string
		config     *Config
		wantStatic bool // Whether staticFileServer should be non-nil
	}{
		{
			name: "no static root",
			config: &Config{
				WebRoot:            "/web",
				StaticRoot:         "",
				SocketDir:          "/tmp/fcgi-sockets",
				ListenAddr:         ":8080",
				DefaultIdleTimeout: 5 * time.Minute,
			},
			wantStatic: false,
		},
		{
			name: "with static root",
			config: &Config{
				WebRoot:            "/web",
				StaticRoot:         tempStaticDir, // Use the temporary directory
				SocketDir:          "/tmp/fcgi-sockets",
				ListenAddr:         ":8080",
				DefaultIdleTimeout: 5 * time.Minute,
			},
			wantStatic: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spawner := NewSpawner(tt.config)

			if spawner.Config != tt.config {
				t.Errorf("NewSpawner() Config field mismatch. Got %v, want %v", spawner.Config, tt.config)
			}
			if spawner.childProcesses == nil {
				t.Errorf("NewSpawner() childProcesses map is nil")
			}
			// Check if staticFileServer is set as expected
			if tt.wantStatic && spawner.staticFileServer == nil {
				t.Errorf("NewSpawner() staticFileServer is nil, but expected to be set")
			}
			if !tt.wantStatic && spawner.staticFileServer != nil {
				t.Errorf("NewSpawner() staticFileServer is not nil, but expected to be nil")
			}
		})
	}
}

func TestCleanupChildProcesses(t *testing.T) {
	// Save original os.Remove and restore after test
	oldOsRemove := osRemove
	defer func() { osRemove = oldOsRemove }()

	// Mock os.Remove to prevent actual file deletion during tests
	osRemove = func(name string) error {
		t.Logf("Mocked os.Remove called for: %s", name)
		return nil
	}

	tests := []struct {
		name               string
		initialChild       map[string]*childProcess
		idleTimeout        time.Duration
		expectedChildCount int
		expectedRemoved    []string // List of socket paths expected to be removed
	}{
		{
			name: "active process, no timeout",
			initialChild: map[string]*childProcess{
				"/app/active.fcgi": {
					cmd: &mockCmd{
						process: &mockProcess{pid: 100, exited: false},
					},
					socketPath: "/tmp/active.sock",
					lastUsed:   time.Now(),
				},
			},
			idleTimeout:        5 * time.Minute,
			expectedChildCount: 1,
			expectedRemoved:    []string{},
		},
		{
			name: "idle process, timeout reached",
			initialChild: map[string]*childProcess{
				"/app/idle.fcgi": {
					cmd: &mockCmd{
						process: &mockProcess{pid: 101, exited: false},
					},
					socketPath: "/tmp/idle.sock",
					lastUsed:   time.Now().Add(-10 * time.Minute), // 10 minutes ago
				},
			},
			idleTimeout:        5 * time.Minute,
			expectedChildCount: 0,
			expectedRemoved:    []string{"/tmp/idle.sock"},
		},
		{
			name: "exited process",
			initialChild: map[string]*childProcess{
				"/app/exited.fcgi": {
					cmd: &mockCmd{
						process: &mockProcess{pid: 102, exited: true, signalErr: syscall.ESRCH}, // Simulate process not found
					},
					socketPath: "/tmp/exited.sock",
					lastUsed:   time.Now(),
				},
			},
			idleTimeout:        5 * time.Minute,
			expectedChildCount: 0,
			expectedRemoved:    []string{"/tmp/exited.sock"},
		},
		{
			name: "idle process, no timeout set",
			initialChild: map[string]*childProcess{
				"/app/no_timeout.fcgi": {
					cmd: &mockCmd{
						process: &mockProcess{pid: 103, exited: false},
					},
					socketPath: "/tmp/no_timeout.sock",
					lastUsed:   time.Now().Add(-10 * time.Minute),
				},
			},
			idleTimeout:        0, // No idle timeout
			expectedChildCount: 1,
			expectedRemoved:    []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{DefaultIdleTimeout: tt.idleTimeout}
			spawner := NewSpawner(cfg)
			spawner.childProcesses = tt.initialChild // Set initial child processes

			// Run cleanup in a goroutine and stop after a short duration
			done := make(chan struct{})
			go func() {
				spawner.cleanupChildProcesses()
				close(done)
			}()

			// Allow cleanup to run for a short period
			time.Sleep(100 * time.Millisecond)

			// Stop the cleanup goroutine (by closing the done channel, if cleanup was designed to listen to it)
			// For now, we'll just let it run for a bit and then check state.
			// A more robust test would involve a context or channel to signal the goroutine to stop.
			// For this test, we'll rely on checking the state after a short sleep.

			spawner.childProcessesMu.Lock()
			if len(spawner.childProcesses) != tt.expectedChildCount {
				t.Errorf("cleanupChildProcesses() got %d child processes, want %d", len(spawner.childProcesses), tt.expectedChildCount)
			}
			spawner.childProcessesMu.Unlock()

			// TODO: Add checks for os.Remove calls if needed, by mocking osRemove to record calls.
		})
	}
}