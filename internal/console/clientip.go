package console

import (
	"net"
	"sync"
)

var (
	clientIPMu sync.RWMutex
	clientIP   string

	localIPsOnce sync.Once
	localIPs     map[string]struct{}
)

// SetClientIP records the remote client's IP address for the active
// session, as captured from DS2's own SSH/web listener. Used to detect when
// a client connecting through DS2's own server is actually on the same
// machine DS2 is running on (e.g. a browser at http://localhost:PORT, or an
// SSH client connecting to 127.0.0.1) -- in that case a file:// hyperlink IS
// meaningful despite the connection going through DS2's own server. CLI
// code never calls this since it never runs behind DS2's own servers.
func SetClientIP(ip string) {
	clientIPMu.Lock()
	clientIP = ip
	clientIPMu.Unlock()
}

// isSameMachineClient reports whether the recorded client IP appears to
// belong to the machine DS2 is running on -- either a loopback address, or
// one that matches one of the host's own network interface addresses. This
// can false-positive if the client's IP happens to coincide with one of the
// host's addresses despite being a different machine (possible behind
// certain NAT/proxy setups, but unusual); that's an accepted tradeoff for
// correctly handling the common case of a user connecting to their own
// machine's SSH/web server from a browser or terminal on that same machine.
func isSameMachineClient() bool {
	clientIPMu.RLock()
	ip := clientIP
	clientIPMu.RUnlock()
	if ip == "" {
		return false
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	if parsed.IsLoopback() {
		return true
	}
	_, ok := localAddresses()[ip]
	return ok
}

func localAddresses() map[string]struct{} {
	localIPsOnce.Do(func() {
		localIPs = make(map[string]struct{})
		addrs, err := net.InterfaceAddrs()
		if err != nil {
			return
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip != nil {
				localIPs[ip.String()] = struct{}{}
			}
		}
	})
	return localIPs
}
