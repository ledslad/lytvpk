package serveraddress

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
)

func TestSteamConnectURLResolvesDomainAndPrefersIPv4(t *testing.T) {
	lookup := func(_ context.Context, host string) ([]net.IPAddr, error) {
		if host != "example.com" {
			t.Fatalf("unexpected host: %s", host)
		}
		return []net.IPAddr{{IP: net.ParseIP("2001:db8::1")}, {IP: net.ParseIP("192.0.2.10")}}, nil
	}
	url, err := steamConnectURL(context.Background(), "example.com:27016", lookup)
	if err != nil || url != "steam://connect/192.0.2.10:27016" {
		t.Fatalf("url=%s err=%v", url, err)
	}
}

func TestSteamConnectURLKeepsNumericAddressesWithoutDNS(t *testing.T) {
	lookup := func(context.Context, string) ([]net.IPAddr, error) {
		t.Fatal("numeric address caused a DNS lookup")
		return nil, nil
	}
	for input, expected := range map[string]string{
		"192.0.2.10":          "steam://connect/192.0.2.10:27015",
		"[2001:db8::1]:27016": "steam://connect/[2001:db8::1]:27016",
	} {
		url, err := steamConnectURL(context.Background(), input, lookup)
		if err != nil || url != expected {
			t.Fatalf("input=%s url=%s err=%v", input, url, err)
		}
	}
}

func TestSteamConnectURLRejectsInvalidAndUnresolvableAddresses(t *testing.T) {
	for _, mode := range []string{"invalid", "empty-dns", "dns-error", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			input := "example.com"
			if mode == "invalid" {
				input = "example.com/invalid"
			}
			if mode == "cancelled" {
				cancel()
			}
			lookup := func(ctx context.Context, _ string) ([]net.IPAddr, error) {
				if mode == "invalid" {
					t.Fatal("invalid input reached DNS lookup")
				}
				if mode == "cancelled" {
					return nil, ctx.Err()
				}
				if mode == "dns-error" {
					return nil, errors.New("DNS unavailable")
				}
				return nil, nil
			}
			url, err := steamConnectURL(ctx, input, lookup)
			if err == nil || url != "" {
				t.Fatalf("url=%s err=%v", url, err)
			}
			if mode == "cancelled" && !errors.Is(err, context.Canceled) {
				t.Fatal("cancellation cause lost", err)
			}
			if mode == "dns-error" && !strings.Contains(err.Error(), "example.com") {
				t.Fatal("error lacks server context", err)
			}
		})
	}
}
