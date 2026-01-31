package appenv

import (
	"fmt"
	"net"
)

// DetectLANNetwork attempts to detect the local LAN network CIDR.
// Mirrors logic of detect_lan_network.sh (but using Go standard library).
//
// Bash logic relies on `ip route get 1` which finds the interface used to reach the internet.
// In Go, we iterate interfaces to find the first likely non-loopback valid private IP.
func DetectLANNetwork() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "192.168.1.0/24"
	}
	for _, i := range ifaces {
		// Skip down interfaces?
		if i.Flags&net.FlagUp == 0 {
			continue
		}
		// Skip loopback
		if i.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := i.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			var mask net.IPMask
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
				mask = v.Mask
			}
			if ip != nil && ip.To4() != nil && !ip.IsLoopback() {
				// We found a valid IPv4 address on a non-loopback interface.
				// This assumes it's the primary LAN.
				// Calculate the network address.
				network := ip.Mask(mask)
				ones, _ := mask.Size()
				return fmt.Sprintf("%s/%d", network.String(), ones)
			}
		}
	}
	return "192.168.1.0/24"
}
