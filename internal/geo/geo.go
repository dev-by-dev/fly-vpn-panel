// Package geo resolves the country and network provider of a client IP
// through Team Cymru's free DNS service (no API key, no local database).
package geo

import (
	"context"
	"fmt"
	"net"
	"strings"
)

// Info is the resolved geo data for one IP.
type Info struct {
	Country  string // ISO 3166-1 alpha-2
	ASN      string // number, без префикса AS
	Provider string // имя сети по ASN
}

// Reverse returns the reversed-octet name used by the DNS zone, or ""
// for addresses the service does not cover here (IPv6).
func Reverse(ip string) string {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return ""
	}
	v4 := parsed.To4()
	if v4 == nil {
		return ""
	}
	return fmt.Sprintf("%d.%d.%d.%d", v4[3], v4[2], v4[1], v4[0])
}

// Lookup resolves country, ASN and provider for ip. Empty Info plus nil
// error means the service had no record (private ranges, IPv6).
func Lookup(ctx context.Context, ip string) (Info, error) {
	rev := Reverse(ip)
	if rev == "" {
		return Info{}, nil
	}
	txt, err := net.DefaultResolver.LookupTXT(ctx, rev+".origin.asn.cymru.com")
	if err != nil || len(txt) == 0 {
		return Info{}, err
	}
	// "ASN | BGP prefix | CC | registry | allocated"
	fields := strings.Split(txt[0], "|")
	if len(fields) < 3 {
		return Info{}, nil
	}
	info := Info{
		ASN:     strings.TrimSpace(fields[0]),
		Country: strings.ToUpper(strings.TrimSpace(fields[2])),
	}
	if info.ASN == "" {
		return info, nil
	}
	asnTXT, err := net.DefaultResolver.LookupTXT(ctx, "AS"+info.ASN+".asn.cymru.com")
	if err == nil && len(asnTXT) > 0 {
		// "ASN | CC | registry | allocated | provider name"
		parts := strings.Split(asnTXT[0], "|")
		if len(parts) >= 5 {
			info.Provider = strings.TrimSpace(parts[4])
		}
	}
	return info, nil
}
