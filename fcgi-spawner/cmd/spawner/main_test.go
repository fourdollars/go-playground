package main

import (
	"flag"
	"os"
	"testing"
	"time"
)

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

// Mock for os.Stat and os.MkdirTemp for future tests
// This is a placeholder for more advanced mocking if needed for other functions.
type mockFileInfo struct {
	isDir bool
}

func (m mockFileInfo) Name() string       { return "mock" }
func (m mockFileInfo) Size() int64        { return 0 }
func (m mockFileInfo) Mode() os.FileMode  { return 0 }
func (m mockFileInfo) ModTime() time.Time { return time.Time{} }
func (m mockFileInfo) IsDir() bool        { return m.isDir }
func (m mockFileInfo) Sys() interface{}   { return nil }

// Example of how to mock os.Stat (not directly used in current tests, but for future reference)
// var osStat = os.Stat
// func TestSomethingWithMockedStat(t *testing.T) {
// 	oldOsStat := osStat
// 	defer func() { osStat = oldOsStat }()

// 	osStat = func(name string) (os.FileInfo, error) {
// 		if name == "/nonexistent" {
// 			return nil, os.ErrNotExist
// 		}
// 		return mockFileInfo{isDir: true}, nil
// 	}

// 	// Now call functions that use os.Stat
// }
