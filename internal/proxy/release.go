package proxy

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/darkyeg/spinup/internal/atomicfile"
	"github.com/darkyeg/spinup/internal/config"
	"github.com/darkyeg/spinup/internal/release"
)

// Project is CLIProxyAPI's GitHub project.
const Project = "router-for-me/CLIProxyAPI"

func Installed(c config.Config) bool {
	_, err := os.Stat(c.ProxyExe())
	return err == nil
}

// InstalledVersion is the version Install last put in place, or "" when unknown.
func InstalledVersion(c config.Config) string {
	data, err := os.ReadFile(versionFile(c))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func versionFile(c config.Config) string { return filepath.Join(c.ProxyDir, "version.txt") }

// Install puts rel's binary in the proxy folder. The old binary is renamed aside, since Windows can't overwrite a running one.
func Install(ctx context.Context, c config.Config, rel release.Release) error {
	archive, err := assetName(rel.Version)
	if err != nil {
		return err
	}
	data, err := rel.Fetch(ctx, archive)
	if err != nil {
		return err
	}
	exeName := filepath.Base(c.ProxyExe())
	files, err := extract(archive, data, map[string]bool{exeName: true, "config.example.yaml": true, "README.md": true, "LICENSE": true})
	if err != nil {
		return err
	}
	if files[exeName] == nil {
		return fmt.Errorf("%s is missing from %s", exeName, archive)
	}
	if err := os.MkdirAll(c.ProxyDir, 0o700); err != nil {
		return err
	}
	for name, content := range files {
		if name == exeName {
			continue
		}
		if err := os.WriteFile(filepath.Join(c.ProxyDir, name), content, 0o644); err != nil {
			return err
		}
	}
	if err := replaceExe(c.ProxyExe(), files[exeName]); err != nil {
		return err
	}
	removeOldExes(c.ProxyExe())
	return os.WriteFile(versionFile(c), []byte(rel.Version+"\n"), 0o644)
}

// removeOldExes deletes binaries renamed aside by earlier installs; one still running stays until next time.
func removeOldExes(exe string) {
	old, _ := filepath.Glob(exe + ".old*")
	for _, f := range old {
		_ = os.Remove(f)
	}
}

func replaceExe(exe string, content []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(exe), ".proxy-*.new")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, 0o755); err != nil {
		return err
	}
	old := fmt.Sprintf("%s.old%d", exe, time.Now().UnixNano())
	if err := atomicfile.Rename(exe, old); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return atomicfile.Rename(name, exe)
	}
	if err := atomicfile.Rename(name, exe); err != nil {
		return errors.Join(err, atomicfile.Rename(old, exe))
	}
	return nil
}

func assetName(version string) (string, error) {
	arch := map[string]string{"amd64": "amd64", "arm64": "aarch64"}[runtime.GOARCH]
	if arch == "" {
		return "", fmt.Errorf("CLIProxyAPI has no build for %s", runtime.GOARCH)
	}
	ext := "tar.gz"
	if runtime.GOOS == "windows" {
		ext = "zip"
	}
	return fmt.Sprintf("CLIProxyAPI_%s_%s_%s.%s", version, runtime.GOOS, arch, ext), nil
}

func extract(archive string, data []byte, keep map[string]bool) (map[string][]byte, error) {
	if strings.HasSuffix(archive, ".zip") {
		return extractZip(data, keep)
	}
	return extractTarGz(data, keep)
}

func extractZip(data []byte, keep map[string]bool) (map[string][]byte, error) {
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	files := map[string][]byte{}
	remaining := uint64(256 << 20)
	for _, f := range z.File {
		name := path.Base(f.Name)
		if !keep[name] || !f.Mode().IsRegular() {
			continue
		}
		if f.UncompressedSize64 > remaining || files[name] != nil {
			return nil, fmt.Errorf("duplicate or oversized CLIProxyAPI archive entry %q", name)
		}
		remaining -= f.UncompressedSize64
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		files[name], err = io.ReadAll(io.LimitReader(rc, int64(f.UncompressedSize64)+1))
		rc.Close()
		if err != nil {
			return nil, err
		}
		if uint64(len(files[name])) != f.UncompressedSize64 {
			return nil, fmt.Errorf("incorrect archive size for %q", name)
		}
	}
	return files, nil
}

func extractTarGz(data []byte, keep map[string]bool) (map[string][]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	files := map[string][]byte{}
	defer gz.Close()
	limited := &io.LimitedReader{R: gz, N: (256 << 20) + 1}
	tr := tar.NewReader(limited)
	for {
		h, err := tr.Next()
		if limited.N == 0 {
			return nil, errors.New("CLIProxyAPI archive exceeds its expanded size limit")
		}
		if err == io.EOF {
			return files, nil
		}
		if err != nil {
			return nil, err
		}
		if name := path.Base(h.Name); keep[name] && h.Typeflag == tar.TypeReg {
			if h.Size >= limited.N || files[name] != nil {
				return nil, fmt.Errorf("duplicate or oversized CLIProxyAPI archive entry %q", name)
			}
			if files[name], err = io.ReadAll(tr); err != nil {
				return nil, err
			}
		}
	}
}
