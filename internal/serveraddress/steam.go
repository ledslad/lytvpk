package serveraddress

import (
	"context"
	"fmt"
	"net"
	"strings"
)

// SteamConnectURL resolves DNS names before handing the address to Steam.
// Steam's connect handler can accept a domain without actually launching the
// game, whereas the same server's numeric address launches correctly.
func SteamConnectURL(ctx context.Context, address string) (string, error) {
	return steamConnectURL(ctx, address, net.DefaultResolver.LookupIPAddr)
}

func steamConnectURL(ctx context.Context, address string, lookup func(context.Context, string) ([]net.IPAddr, error)) (string, error) {
	normalized, err := Normalize(address)
	if err != nil {
		return "", err
	}
	host, port, err := net.SplitHostPort(normalized)
	if err != nil {
		return "", err
	}
	ipText, _, _ := strings.Cut(host, "%")
	if net.ParseIP(ipText) != nil {
		return "steam://connect/" + normalized, nil
	}
	addresses, err := lookup(ctx, host)
	if err != nil {
		return "", fmt.Errorf("解析服务器域名 %s 失败，请检查网络或改用 IP 地址：%w", host, err)
	}
	// Prefer IPv4 for the Source engine, even when DNS lists IPv6 first.
	var fallback string
	for _, candidate := range addresses {
		if ip := candidate.IP.To4(); ip != nil {
			return "steam://connect/" + net.JoinHostPort(ip.String(), port), nil
		}
		if ip := candidate.IP.To16(); ip != nil && fallback == "" {
			fallback = ip.String()
			if candidate.Zone != "" {
				fallback += "%" + candidate.Zone
			}
		}
	}
	if fallback != "" {
		return "steam://connect/" + net.JoinHostPort(fallback, port), nil
	}
	return "", fmt.Errorf("服务器域名 %s 未返回可用的 IP 地址", host)
}
