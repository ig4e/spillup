// Package mcp manages portable MCP definitions and explicit per-agent approvals.
package mcp

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/darkyeg/spinup/internal/library"
)

const Manifest = library.MCPServers

type Definition struct {
	Name       string   `json:"name"`
	Transport  string   `json:"transport"`
	Command    string   `json:"command,omitempty"`
	Args       []string `json:"args,omitempty"`
	URL        string   `json:"url,omitempty"`
	SecretKeys []string `json:"secretKeys,omitempty"`
}

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,62}$`)
var envPattern = regexp.MustCompile(`^[A-Z_][A-Z0-9_]{0,63}$`)
var binaryPattern = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9_.+-]{0,126}$`)

func (d Definition) Validate() error {
	if !namePattern.MatchString(d.Name) {
		return errors.New("use a lowercase MCP name starting with a letter, up to 63 characters")
	}
	switch d.Transport {
	case "http":
		u, err := url.Parse(d.URL)
		if err != nil || len(d.URL) > 2048 || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || strings.Contains(d.URL, "#") {
			return errors.New("enter an HTTPS server URL without user info, a query or a fragment")
		}
		if d.Command != "" || len(d.Args)+len(d.SecretKeys) > 0 {
			return errors.New("HTTP servers use native client authentication")
		}
	case "stdio":
		if !binaryPattern.MatchString(d.Command) || d.URL != "" || len(d.Args) > 64 || len(d.SecretKeys) > 32 {
			return errors.New("enter a portable executable name, up to 64 arguments and 32 secret keys")
		}
		for _, a := range d.Args {
			if len(a) > 2048 || strings.IndexFunc(a, unicode.IsControl) >= 0 {
				return errors.New("arguments must be single-line text up to 2048 bytes")
			}
		}
		for i, k := range d.SecretKeys {
			if !envPattern.MatchString(k) || slices.Contains(d.SecretKeys[:i], k) {
				return errors.New("use unique uppercase environment variable names for secret keys")
			}
		}
	default:
		return errors.New("choose stdio or HTTP transport")
	}
	return nil
}

func (d Definition) Revision() string {
	data, _ := json.Marshal(d)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

type catalog struct {
	Servers []Definition `json:"servers"`
}

func parse(data []byte) (catalog, error) {
	var c catalog
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if len(data) > 1<<20 || dec.Decode(&c) != nil || dec.Decode(new(json.RawMessage)) != io.EOF || c.Servers == nil || len(c.Servers) > 256 {
		return catalog{}, errors.New("invalid MCP server catalog")
	}
	seen := map[string]bool{}
	for _, d := range c.Servers {
		if err := d.Validate(); err != nil {
			return catalog{}, err
		}
		if seen[d.Name] {
			return catalog{}, errors.New("duplicate MCP server name")
		}
		seen[d.Name] = true
	}
	return c, nil
}

func ValidateManifest(data []byte) error { _, err := parse(data); return err }
