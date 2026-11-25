package main

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"time"
	"unicode/utf8"

	"github.com/darkyeg/spinup/internal/api"
	"github.com/darkyeg/spinup/internal/config"
	"github.com/darkyeg/spinup/internal/mcpvault"
	"github.com/darkyeg/spinup/internal/tailnet"
	"github.com/darkyeg/spinup/internal/webui"
)

var errMCPPeer = errors.New("select an online, verified Spillup machine on your Tailscale account")

func mcpPeer(st tailnet.Status, name string) (tailnet.Node, bool) {
	if !st.Running || st.Self.ID == "" || !tailnet.ValidName(name) || name == st.Self.Name {
		return tailnet.Node{}, false
	}
	var found tailnet.Node
	for _, n := range st.Peers {
		if n.Name == name {
			if found.ID != "" || !st.SameOwner(n) || n.ID == "" || !n.Online || n.Expired {
				return tailnet.Node{}, false
			}
			found = n
		}
	}
	return found, found.ID != ""
}

// Bind consent to both stable node IDs and both selected names, including after a rename or reinstall.
func mcpBinding(st tailnet.Status, n tailnet.Node) string {
	return fmt.Sprintf("%q %q %q %q", st.Self.ID, st.Self.Name, n.ID, n.Name)
}

func mcpSendingPeer(ctx context.Context, name string) (tailnet.Status, tailnet.Node, string, api.Client, error) {
	cfg, err := config.Load()
	if err != nil {
		return tailnet.Status{}, tailnet.Node{}, "", api.Client{}, errMCPPeer
	}
	secrets, err := config.LoadSecrets(cfg)
	if err != nil {
		return tailnet.Status{}, tailnet.Node{}, "", api.Client{}, errMCPPeer
	}
	st, err := (tailnet.CLI{Bin: cfg.Tailscale}).Status(ctx)
	n, ok := mcpPeer(st, name)
	if err != nil || !ok {
		return st, n, "", api.Client{}, errMCPPeer
	}
	base, err := proveFleetNode(ctx, n, cfg.Port, secrets.APIKey)
	if err != nil {
		return st, n, "", api.Client{}, errMCPPeer
	}
	return st, n, base + webui.BridgePrefix + "/api/host/mcp/", api.Client{HTTP: directMachineClient, APIKey: secrets.APIKey, MaxResponseBytes: 1024}, nil
}

func (cliBackend) MCPSecret(ctx context.Context, op string, in webui.MCPSecretRequest) error {
	if err := mcpvault.Validate(in.Server, in.Key); err != nil {
		return err
	}
	if op != "forget" {
		if err := (cliBackend{}).MCP().DeclaresSecret(in.Server, in.Key); err != nil {
			return err
		}
	}
	store := mcpvault.Store{Dir: config.StateDir()}
	switch op {
	case "secret":
		return store.Set(ctx, in.Server, in.Key, in.Value)
	case "forget":
		return store.Forget(ctx, in.Server, in.Key)
	case "allow", "share":
		ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
		defer cancel()
		st, peer, endpoint, client, err := mcpSendingPeer(ctx, in.Machine)
		if err != nil {
			return err
		}
		if op == "allow" {
			return store.Allow(ctx, in.Server, in.Key, mcpBinding(st, peer))
		}
		ref := webui.MCPShareRef{Server: in.Server, Key: in.Key, Source: st.Self.Name, Target: peer.Name}
		return sendMCPSecret(ctx, client, endpoint, ref, store)
	}
	return errors.New("unknown vault operation")
}

func sendMCPSecret(ctx context.Context, client api.Client, endpoint string, ref webui.MCPShareRef, store mcpvault.Store) error {
	var response struct {
		Permit string `json:"permit"`
	}
	if err := client.Post(ctx, endpoint+"permit", ref, &response); err != nil || len(response.Permit) < 20 || len(response.Permit) > 128 {
		return errors.New("approve this sender, server and key on the receiving machine, then retry")
	}
	value, err := store.Get(ref.Server, ref.Key)
	if err != nil {
		return errors.New("the selected MCP secret is unavailable on this machine")
	}
	body := webui.MCPTransfer{MCPShareRef: ref, Permit: response.Permit, Value: []byte(value)}
	if err := client.Post(ctx, endpoint+"receive", body, nil); err != nil {
		return errors.New("the transfer could not be confirmed; check the key on the receiving machine")
	}
	return nil
}

func mcpCaller(st tailnet.Status, remote string, ref webui.MCPShareRef) (string, error) {
	addr, err := netip.ParseAddrPort(remote)
	n, ok := mcpPeer(st, ref.Source)
	if err != nil || !ok || ref.Target != st.Self.Name {
		return "", errMCPPeer
	}
	for _, raw := range []string{n.IP, n.IPv6} {
		if ip, err := netip.ParseAddr(raw); err == nil && ip.Unmap() == addr.Addr().Unmap() {
			return mcpBinding(st, n), nil
		}
	}
	return "", errMCPPeer
}

func mcpSource(ctx context.Context, remote string, ref webui.MCPShareRef) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	st, err := (cliBackend{}).Tailnet(ctx)
	if err != nil {
		return "", errMCPPeer
	}
	return mcpCaller(st, remote, ref)
}

func (cliBackend) MCPPermit(ctx context.Context, remote string, ref webui.MCPShareRef) (string, error) {
	source, err := mcpSource(ctx, remote, ref)
	if err != nil {
		return "", err
	}
	if err := (cliBackend{}).MCP().DeclaresSecret(ref.Server, ref.Key); err != nil {
		return "", err
	}
	return (mcpvault.Store{Dir: config.StateDir()}).Permit(ref.Server, ref.Key, source)
}

func (cliBackend) MCPReceive(ctx context.Context, remote string, req webui.MCPTransfer) error {
	source, err := mcpSource(ctx, remote, req.MCPShareRef)
	if err != nil {
		return err
	}
	if err := (cliBackend{}).MCP().DeclaresSecret(req.Server, req.Key); err != nil {
		return err
	}
	if !utf8.Valid(req.Value) {
		return errors.New("secret must be valid text")
	}
	return (mcpvault.Store{Dir: config.StateDir()}).Receive(ctx, req.Server, req.Key, source, req.Permit, string(req.Value))
}
