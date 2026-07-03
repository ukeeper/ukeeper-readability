package extractor

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"syscall"
	"time"
)

// errBlockedAddress is returned when a fetch targets a non-public address and private-network
// blocking is enabled. the check runs at connect time against the actual resolved IP, so it also
// defends against DNS rebinding and redirects that point back at internal hosts.
var errBlockedAddress = errors.New("connection to non-public address blocked")

// isBlockedIP reports whether ip is outside the public routable range and must not be dialed.
func isBlockedIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast()
}

// safeControl is a net.Dialer Control hook that rejects connections to non-public addresses.
func safeControl(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("%w: cannot parse address %q", errBlockedAddress, host)
	}
	if isBlockedIP(ip) {
		return fmt.Errorf("%w: %s", errBlockedAddress, ip)
	}
	return nil
}

// safeTransport returns an http.Transport whose dialer blocks non-public addresses.
func safeTransport(timeout time.Duration) *http.Transport {
	dialer := &net.Dialer{Timeout: timeout, Control: safeControl}
	tr, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return &http.Transport{DialContext: dialer.DialContext}
	}
	cloned := tr.Clone()
	cloned.DialContext = dialer.DialContext
	return cloned
}
