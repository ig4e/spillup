package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/darkyeg/spinup/internal/mcp"
	"github.com/darkyeg/spinup/internal/webui"
)

type mcpCmd struct {
	List      mcpListCmd    `cmd:"" default:"1" help:"List definitions, sharing and locally approved agents."`
	Import    mcpImportCmd  `cmd:"" help:"Save a JSON server definition locally, or publish it with --shared."`
	Remove    mcpRemoveCmd  `cmd:"" help:"Remove the selected definition after disabling its agents."`
	Sharing   mcpSharingCmd `cmd:"" help:"Detach a local copy, publish it, or follow the shared version."`
	Enable    mcpEnableCmd  `cmd:"" help:"Approve the current definition for a local agent."`
	Disable   mcpDisableCmd `cmd:"" help:"Remove this server from a local agent."`
	Secret    mcpSecretCmd  `cmd:"" help:"Set, forget, allow or share one named secret."`
	RunServer mcpRunCmd     `cmd:"" name:"run" hidden:""`
}
type mcpListCmd struct{}

func (mcpListCmd) Run() error {
	v, err := (cliBackend{}).MCP().List()
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(v)
}

type mcpImportCmd struct {
	File   string `arg:"" help:"JSON file containing one definition."`
	Shared bool   `help:"Publish this definition to your other machines."`
}

func (c mcpImportCmd) Run() error {
	f, err := os.Open(c.File)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 65537))
	if err != nil || len(data) > 65536 {
		return errors.New("definition must be at most 64 KiB")
	}
	var def mcp.Definition
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if dec.Decode(&def) != nil || dec.Decode(new(json.RawMessage)) != io.EOF {
		return errors.New("expected one MCP server definition")
	}
	scope := "local"
	if c.Shared {
		catchUpLibrary()
		scope = "shared"
	}
	return (cliBackend{}).MCP().Save(context.Background(), def, scope)
}

type mcpRemoveCmd struct {
	Name string `arg:""`
}

func (c mcpRemoveCmd) Run() error {
	catchUpLibrary()
	return (cliBackend{}).MCP().Remove(context.Background(), c.Name)
}

type mcpSharingCmd struct {
	Name   string `arg:""`
	Action string `arg:"" enum:"detach,publish,follow,unpublish"`
}
type mcpAgentArgs struct {
	Name   string `arg:""`
	Target string `arg:"" enum:"claude,codex"`
}
type mcpEnableCmd mcpAgentArgs
type mcpDisableCmd mcpAgentArgs
type mcpRunCmd mcpAgentArgs

func findMCP(name string) (mcp.Server, error) {
	v, err := (cliBackend{}).MCP().List()
	if err != nil {
		return mcp.Server{}, err
	}
	for _, s := range v.Servers {
		if s.Name == name {
			return s, nil
		}
	}
	return mcp.Server{}, errors.New("unknown MCP server")
}
func (c mcpEnableCmd) Run() error {
	row, err := findMCP(c.Name)
	if err != nil {
		return err
	}
	return (cliBackend{}).MCP().Target(context.Background(), c.Name, c.Target, row.Revision, true)
}
func (c mcpDisableCmd) Run() error {
	return (cliBackend{}).MCP().Target(context.Background(), c.Name, c.Target, "", false)
}
func (c mcpSharingCmd) Run() error {
	catchUpLibrary()
	row, err := findMCP(c.Name)
	if err != nil {
		return err
	}
	return (cliBackend{}).MCP().Sharing(context.Background(), c.Name, c.Action, row.Revision)
}
func (c mcpRunCmd) Run() error {
	return (cliBackend{}).MCP().Run(context.Background(), c.Name, c.Target)
}

type mcpSecretCmd struct {
	Action  string `arg:"" enum:"set,forget,allow,share"`
	Name    string `arg:""`
	Key     string `arg:""`
	Machine string `arg:"" optional:"" help:"Verified peer for allow and share."`
}

func (c mcpSecretCmd) Run() error {
	req := webui.MCPSecretRequest{Server: c.Name, Key: c.Key, Machine: c.Machine}
	if (c.Action == "allow" || c.Action == "share") != (c.Machine != "") {
		return errors.New("supply a machine only for allow or share")
	}
	op := c.Action
	if c.Action == "set" {
		op = "secret"
		var value []byte
		var err error
		if term.IsTerminal(int(os.Stdin.Fd())) {
			fmt.Fprint(os.Stderr, "Secret value: ")
			value, err = term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Fprintln(os.Stderr)
		} else {
			value, err = io.ReadAll(io.LimitReader(os.Stdin, 16386))
		}
		if err != nil {
			return errors.New("could not read secret")
		}
		req.Value = strings.TrimSuffix(strings.TrimSuffix(string(value), "\n"), "\r")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return (cliBackend{}).MCPSecret(ctx, op, req)
}
