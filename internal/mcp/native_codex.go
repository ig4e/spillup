package mcp

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"github.com/darkyeg/spinup/internal/host"
)

// Codex's MCP list/get omits tool approval fields. The native app-server's
// config API exposes the complete user layer and supports conditional edits to
// one key. It also registers HTTP URLs without opening an interactive browser.
func openCodex(ctx context.Context, tool nativeTool, dir, alias string) (nativeConfig, error) {
	home := host.CodexHome()
	if !filepath.IsAbs(home) {
		return nativeConfig{}, errNativeRead
	}
	cmd := tool.command(ctx, dir, "app-server", "--listen", "stdio://")
	in, err := cmd.StdinPipe()
	if err != nil {
		return nativeConfig{}, errNativeRead
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		in.Close()
		return nativeConfig{}, errNativeRead
	}
	if err := cmd.Start(); err != nil {
		in.Close()
		out.Close()
		return nativeConfig{}, errNativeRead
	}
	stop := context.AfterFunc(ctx, func() { in.Close(); out.Close() })
	closeRPC := func() { stop(); in.Close(); out.Close(); _ = cmd.Process.Kill(); _ = cmd.Wait() }
	ok := false
	defer func() {
		if !ok {
			closeRPC()
		}
	}()
	write := json.NewEncoder(in)
	read := json.NewDecoder(io.LimitReader(out, 2*maxNativeJSON))
	id := 0
	call := func(method string, params any, result any) bool {
		id++
		if write.Encode(map[string]any{"id": id, "method": method, "params": params}) != nil {
			return false
		}
		for {
			var reply struct {
				ID     int
				Result json.RawMessage
			}
			if read.Decode(&reply) != nil {
				return false
			}
			if reply.ID != id {
				continue
			} // notifications and unrelated messages
			if len(reply.Result) == 0 {
				return false
			}
			return result == nil || json.Unmarshal(reply.Result, result) == nil
		}
	}
	if !call("initialize", map[string]any{"clientInfo": map[string]string{"name": "spillup", "version": "1"}}, nil) ||
		write.Encode(map[string]any{"method": "initialized", "params": map[string]any{}}) != nil {
		return nativeConfig{}, errNativeRead
	}
	var response struct {
		Layers []struct {
			Name struct {
				Type    string
				File    string
				Profile *string
			}
			Version string
			Config  struct {
				Servers map[string]json.RawMessage `json:"mcp_servers"`
			}
		}
	}
	if !call("config/read", map[string]bool{"includeLayers": true}, &response) {
		return nativeConfig{}, errNativeRead
	}
	userFile := filepath.Join(home, "config.toml")
	var current json.RawMessage
	var version string
	for _, layer := range response.Layers {
		entry := layer.Config.Servers[alias]
		if layer.Name.Type == "user" && layer.Name.Profile == nil && sameConfigFile(layer.Name.File, userFile) {
			if version != "" {
				return nativeConfig{}, errNativeRead
			}
			version = layer.Version
			current = entry
		} else if entry != nil {
			return nativeConfig{}, errCollision
		}
	}
	if version == "" {
		return nativeConfig{}, errNativeRead
	}
	apply := func(_ context.Context, entry map[string]any) error {
		var result struct {
			Status   string
			FilePath string
		}
		params := map[string]any{"filePath": userFile, "keyPath": "mcp_servers." + alias, "mergeStrategy": "replace", "value": entry, "expectedVersion": version}
		if !call("config/value/write", params, &result) || result.Status != "ok" || !sameConfigFile(result.FilePath, userFile) {
			return errNativeWrite
		}
		return nil
	}
	ok = true
	return nativeConfig{current: current, apply: apply, close: closeRPC}, nil
}

func sameConfigFile(a, b string) bool {
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	one, err := os.Stat(a)
	if err != nil {
		return false
	}
	two, err := os.Stat(b)
	return err == nil && os.SameFile(one, two)
}
