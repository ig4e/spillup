package mcp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/darkyeg/spinup/internal/mcpvault"
)

// Run attaches an approved stdio child to the native client's pipes.
func (m Manager) Run(ctx context.Context, name, target string) error {
	if !slices.Contains(targets, target) || !namePattern.MatchString(name) {
		return errors.New("invalid MCP server or agent")
	}
	approved, err := readCatalog(m.approvalPath(target))
	if err != nil {
		return err
	}
	pinned, ok := approved[name]
	if !ok {
		return errors.New("enable this MCP server on this machine first")
	}
	c, err := m.catalogs()
	if err != nil {
		return err
	}
	d, _, ok := c.resolve(name)
	if !ok || d.Transport != "stdio" || d.Revision() != pinned.Revision() {
		return errors.New("MCP definition changed; review it on this machine before enabling")
	}
	command := exec.CommandContext(ctx, d.Command, d.Args...)
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	command.Env = []string{}
	// Supply the OS runtime environment and only the credentials declared for this server.
	for _, key := range []string{"PATH", "HOME", "USERPROFILE", "SYSTEMROOT", "WINDIR", "COMSPEC", "PATHEXT", "LOCALAPPDATA", "APPDATA", "TEMP", "TMP", "TMPDIR", "LANG", "LC_ALL", "USER", "LOGNAME"} {
		if value, ok := os.LookupEnv(key); ok && !slices.Contains(d.SecretKeys, key) {
			command.Env = append(command.Env, key+"="+value)
		}
	}
	for _, key := range d.SecretKeys {
		value, err := (mcpvault.Store{Dir: m.State}).Get(name, key)
		if err != nil {
			return fmt.Errorf("set the MCP secret %s on this machine before starting this server", key)
		}
		command.Env = append(command.Env, strings.Join([]string{key, value}, "="))
	}
	if command.Err != nil {
		return errors.New("the approved MCP executable was not found")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return runStdio(command)
}
