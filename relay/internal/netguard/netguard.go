// Package netguard keeps the relay from sending requests into the network it runs in.
//
// Endpoint URLs come from customers. Without a guard, a customer can register
// http://169.254.169.254/ or http://10.0.0.5:9200/ and read the answers in the
// attempt log. The check runs on the address the dialer is about to connect to,
// after DNS, so a name that resolves to a private address is caught too, also when
// it resolved to a public one a moment earlier.
package netguard

import (
	"fmt"
	"net"
	"net/netip"
	"syscall"
)

// blocked lists the ranges that are not checked by the netip.Addr predicates.
var blocked = []struct {
	prefix netip.Prefix
	name   string
}{
	{netip.MustParsePrefix("0.0.0.0/8"), "this network"},
	{netip.MustParsePrefix("100.64.0.0/10"), "carrier-grade NAT"},
	{netip.MustParsePrefix("192.0.0.0/24"), "IETF protocol assignments"},
	{netip.MustParsePrefix("192.0.2.0/24"), "documentation"},
	{netip.MustParsePrefix("198.18.0.0/15"), "benchmarking"},
	{netip.MustParsePrefix("198.51.100.0/24"), "documentation"},
	{netip.MustParsePrefix("203.0.113.0/24"), "documentation"},
	{netip.MustParsePrefix("240.0.0.0/4"), "reserved"},
	{netip.MustParsePrefix("2001:db8::/32"), "documentation"},
}

// nat64 is the well-known NAT64 prefix: the last 32 bits are an IPv4 address.
var nat64 = netip.MustParsePrefix("64:ff9b::/96")

// Check returns an error when addr is not a public unicast address.
func Check(addr netip.Addr) error {
	addr = addr.Unmap()
	if nat64.Contains(addr) {
		b := addr.As16()
		if err := Check(netip.AddrFrom4([4]byte(b[12:]))); err != nil {
			return fmt.Errorf("%s maps to %w", addr, err)
		}
		return nil
	}
	var name string
	switch {
	case !addr.IsValid():
		return fmt.Errorf("invalid address")
	case addr.IsLoopback():
		name = "loopback"
	case addr.IsPrivate():
		name = "private network"
	case addr.IsLinkLocalUnicast():
		name = "link-local, cloud metadata lives here"
	case addr.IsUnspecified():
		name = "unspecified"
	case addr.IsMulticast(), addr.IsLinkLocalMulticast(), addr.IsInterfaceLocalMulticast():
		name = "multicast"
	case addr == netip.AddrFrom4([4]byte{255, 255, 255, 255}):
		name = "broadcast"
	default:
		for _, b := range blocked {
			if b.prefix.Contains(addr) {
				name = b.name
				break
			}
		}
	}
	if name != "" {
		return fmt.Errorf("%s is not a public address (%s)", addr, name)
	}
	return nil
}

// Control is a net.Dialer Control function that refuses connections to addresses
// Check rejects.
func Control(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return err
	}
	if err := Check(addr); err != nil {
		return fmt.Errorf("blocked: %w", err)
	}
	return nil
}
