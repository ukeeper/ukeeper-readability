package extractor

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"syscall"
	"time"
)

// errBlockedAddress is returned when a fetch targets a non-public address and private-network
// blocking is enabled. the check runs at connect time against the actual resolved IP, so it also
// defends against DNS rebinding and redirects that point back at internal hosts.
var errBlockedAddress = errors.New("connection to non-public address blocked")

// extraBlockedCIDRs are IANA special-purpose ranges that the net.IP helpers below do not classify
// as private/loopback/link-local but which are still not public, routable destinations. Blocking
// them closes SSRF paths into CGNAT, benchmarking, documentation, reserved and broadcast space.
var extraBlockedCIDRs = parseCIDRs(
	"0.0.0.0/8",          // RFC1122 "this network" (IsUnspecified only covers 0.0.0.0 itself)
	"100.64.0.0/10",      // RFC6598 carrier-grade NAT
	"192.0.0.0/24",       // RFC6890 IETF protocol assignments
	"192.0.2.0/24",       // TEST-NET-1 documentation
	"198.18.0.0/15",      // RFC2544 benchmarking
	"198.51.100.0/24",    // TEST-NET-2 documentation
	"203.0.113.0/24",     // TEST-NET-3 documentation
	"240.0.0.0/4",        // RFC1112 reserved (former class E)
	"255.255.255.255/32", // limited broadcast
	"64:ff9b::/96",       // RFC6052 NAT64
	"100::/64",           // RFC6666 discard-only
	"2001:db8::/32",      // IPv6 documentation
)

func parseCIDRs(cidrs ...string) []*net.IPNet {
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		if _, n, err := net.ParseCIDR(c); err == nil {
			nets = append(nets, n)
		}
	}
	return nets
}

// isBlockedIP reports whether ip is outside the public routable range and must not be dialed.
func isBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return true
	}
	for _, n := range extraBlockedCIDRs {
		if n.Contains(ip) {
			return true
		}
	}
	return false
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

// validatePublicHost rejects reqURL when its host is, or resolves to, a non-public address. It
// enforces the SSRF policy uniformly across all retrievers, including Cloudflare Browser Rendering
// which fetches remotely and so is not covered by the connect-time dialer guard.
func validatePublicHost(ctx context.Context, reqURL string) error {
	u, err := url.Parse(reqURL)
	if err != nil {
		return err
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("%w: no host in %q", errBlockedAddress, reqURL)
	}
	if ip := net.ParseIP(host); ip != nil {
		if isBlockedIP(ip) {
			return fmt.Errorf("%w: %s", errBlockedAddress, ip)
		}
		return nil
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("resolve %q: %w", host, err)
	}
	for _, ipa := range ips {
		if isBlockedIP(ipa.IP) {
			return fmt.Errorf("%w: %s resolves to %s", errBlockedAddress, host, ipa.IP)
		}
	}
	return nil
}

// safeTransport returns an http.Transport whose dialer blocks non-public addresses. Proxy support
// is disabled: an env proxy would make the dialer validate the proxy's IP instead of the real
// destination, defeating the guard. As a consequence, deployments that require HTTP_PROXY/HTTPS_PROXY
// for outbound access must run with --allow-private-networks (which keeps the default proxy-aware
// transport) and rely on network-level egress controls instead.
func safeTransport(timeout time.Duration) *http.Transport {
	dialer := &net.Dialer{Timeout: timeout, Control: safeControl}
	tr, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return &http.Transport{DialContext: dialer.DialContext, Proxy: nil}
	}
	cloned := tr.Clone()
	cloned.Proxy = nil
	cloned.DialContext = dialer.DialContext
	return cloned
}
