package server

import (
	"context"
	"errors"
	"github.com/ane/ane/internal/core"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

func (a *App) validateCallbackURL(raw string) error {
	if err := core.ValidateURL(raw); err != nil {
		return err
	}
	u, _ := url.Parse(raw)
	if !a.cfg.AllowPrivateCallbacks {
		host := strings.ToLower(u.Hostname())
		if host == "localhost" || strings.HasSuffix(host, ".localhost") {
			return errors.New("回调不允许访问本机地址")
		}
		if ip, err := netip.ParseAddr(host); err == nil && !publicIP(ip) {
			return errors.New("回调不允许访问内网或保留地址")
		}
	}
	return nil
}
func publicIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	// Include metadata, CGNAT, documentation, benchmark and transition ranges.
	for _, s := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32", "2001::/32", "2002::/16", "64:ff9b::/96", "64:ff9b:1::/48"} {
		if netip.MustParsePrefix(s).Contains(ip) {
			return false
		}
	}
	return true
}
func (a *App) newCallbackClient() *http.Client {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{Proxy: nil, MaxIdleConns: 10, IdleConnTimeout: 60 * time.Second, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 8 * time.Second, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return nil, err
		}
		if len(ips) == 0 {
			return nil, errors.New("callback host resolved no addresses")
		}
		for _, ip := range ips {
			if !a.cfg.AllowPrivateCallbacks && !publicIP(ip) {
				return nil, errors.New("callback resolved to a private or reserved address")
			}
		}
		var last error
		for _, ip := range ips {
			conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if err == nil {
				return conn, nil
			}
			last = err
		}
		return nil, last
	}}
	return &http.Client{Transport: transport, Timeout: 12 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
}
