package appenv

import (
	"DockSTARTer2/internal/system"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// DetectDockerSocket attempts to find the docker socket on the system without using command dependencies.
// It prioritizes DOCKER_HOST, active sockets reachable via API, and then common filesystem locations.
func DetectDockerSocket() string {
	// 0. Check DOCKER_HOST env var first (the "gold standard")
	if dockerHost := os.Getenv("DOCKER_HOST"); dockerHost != "" {
		if strings.HasPrefix(dockerHost, "unix://") {
			return strings.TrimPrefix(dockerHost, "unix://")
		}
		if strings.HasPrefix(dockerHost, "tcp://") {
			return dockerHost
		}
	}

	if runtime.GOOS == "windows" {
		return "//./pipe/docker_engine"
	}

	// 1. Check Docker Contexts (API-level detection without calling docker command)
	if contextSocket := detectContextSocket(); contextSocket != "" {
		return contextSocket
	}

	// List of potential sockets to check
	potentialSockets := []string{
		"/var/run/docker.sock",
		"/run/docker.sock",
	}

	// Add rootless sockets
	if xdgRuntimeDir := os.Getenv("XDG_RUNTIME_DIR"); xdgRuntimeDir != "" {
		potentialSockets = append(potentialSockets, filepath.Join(xdgRuntimeDir, "docker.sock"))
	}
	puid, _ := system.GetIDs()
	potentialSockets = append(potentialSockets, fmt.Sprintf("/run/user/%d/docker.sock", puid))

	// 2. "API" check: Try to connect to each potential socket to find the "real" active one
	for _, socket := range potentialSockets {
		if isSocketActive(socket) {
			return socket
		}
	}

	// 3. Fallback: return the first one that exists on disk even if not responsive
	for _, socket := range potentialSockets {
		if fileExists(socket) {
			return socket
		}
	}

	// Final fallback to standard
	return "/var/run/docker.sock"
}

// detectContextSocket parses ~/.docker/config.json and context metadata
func detectContextSocket() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	configPath := filepath.Join(home, ".docker", "config.json")
	if !fileExists(configPath) {
		return ""
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return ""
	}

	var config struct {
		CurrentContext string `json:"currentContext"`
	}
	if err := json.Unmarshal(data, &config); err != nil || config.CurrentContext == "" || config.CurrentContext == "default" {
		return ""
	}

	// Context metadata is stored in a folder named with the sha256 of the context name
	hash := sha256.Sum256([]byte(config.CurrentContext))
	hashStr := hex.EncodeToString(hash[:])
	contextMetaPath := filepath.Join(home, ".docker", "contexts", "meta", hashStr, "meta.json")

	if !fileExists(contextMetaPath) {
		return ""
	}

	metaData, err := os.ReadFile(contextMetaPath)
	if err != nil {
		return ""
	}

	var meta struct {
		Endpoints struct {
			Docker struct {
				Host string `json:"Host"`
			} `json:"docker"`
		} `json:"Endpoints"`
	}
	if err := json.Unmarshal(metaData, &meta); err != nil {
		return ""
	}

	host := meta.Endpoints.Docker.Host
	if strings.HasPrefix(host, "unix://") {
		socket := strings.TrimPrefix(host, "unix://")
		if isSocketActive(socket) {
			return socket
		}
	}

	return ""
}

// isSocketActive checks if the socket is actually responsive (API check)
func isSocketActive(path string) bool {
	if !fileExists(path) {
		return false
	}
	// Try to connect to the unix socket
	conn, err := net.DialTimeout("unix", path, 100*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
