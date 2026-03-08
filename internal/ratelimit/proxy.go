package ratelimit

import (
	"fmt"
	"net"
)

// parseCIDRs parses a list of CIDR strings into net.IPNet objects.
func parseCIDRs(cidrs []string) ([]*net.IPNet, error) {
	var nets []*net.IPNet
	for _, cidr := range cidrs {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR %q: %w", cidr, err)
		}
		nets = append(nets, ipNet)
	}
	return nets, nil
}
