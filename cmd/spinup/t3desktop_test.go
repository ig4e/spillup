package main

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/darkyeg/spinup/internal/api"
	"github.com/darkyeg/spinup/internal/tailnet"
)

type desktopFixtureTransport func(*http.Request) (*http.Response, error)

func (f desktopFixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestT3DesktopPairsOnlyTheVerifiedTailnetEnvironment(t *testing.T) {
	const base = "http://100.64.0.8:8317"
	const address = "http://laptop.tail1234.ts.net:4000"
	const code = "fixture/credential+test"
	for _, match := range []bool{true, false} {
		minted := 0
		httpClient := &http.Client{Transport: desktopFixtureTransport(func(r *http.Request) (*http.Response, error) {
			data := ""
			switch r.URL.String() {
			case base + "/spinup/ui/api/t3":
				if r.Header.Get(api.APIKeyHeader) != "test-api" {
					t.Fatal("missing bridge authentication")
				}
				data = `{"installed":true,"port":4000,"environment_id":"expected","address":"http://untrusted.example:4000"}`
			case address + "/.well-known/t3/environment":
				if r.Header.Get(api.APIKeyHeader) != "" || r.Header.Get(api.KeyHeader) != "" {
					t.Fatal("fleet key sent to T3")
				}
				data = `{"environmentId":"other"}`
				if match {
					data = `{"environmentId":"expected"}`
				}
			case base + "/spinup/ui/api/t3/pair":
				if r.Method != "POST" || r.Header.Get(api.APIKeyHeader) != "test-api" {
					t.Fatal("invalid mint request")
				}
				minted++
				data = `{"code":"fixture/credential+test"}`
			default:
				t.Fatal("unexpected endpoint", r.URL)
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(data)), Header: make(http.Header)}, nil
		})}
		node := tailnet.Node{Name: "laptop", IP: "100.64.0.8", DNSName: "laptop.tail1234.ts.net"}
		link, err := desktopPairing(context.Background(), tailnet.Status{MagicDNS: true, Self: tailnet.Node{Name: "desktop"}}, node, base, "test-api", httpClient)
		if !match {
			if err == nil || minted != 0 || link != "" {
				t.Fatal("mismatch minted a desktop credential")
			}
			continue
		}
		if err != nil || minted != 1 {
			t.Fatal("failed to mint", err)
		}
		outer, err := url.Parse(link)
		if err != nil || outer.Scheme != "t3code" || outer.Host != "pair" {
			t.Fatal("invalid native link")
		}
		pair, err := url.Parse(outer.Query().Get("pairingUrl"))
		if err != nil || pair.Scheme+"://"+pair.Host != address || pair.Path != "/pair" {
			t.Fatal("wrong pairing target")
		}
		token, err := url.ParseQuery(pair.EscapedFragment())
		if err != nil || token.Get("token") != code {
			t.Fatal("invalid pairing token encoding")
		}
	}
}
