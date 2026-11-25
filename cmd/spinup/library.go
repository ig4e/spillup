package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/darkyeg/spinup/internal/agentconfig"
	"github.com/darkyeg/spinup/internal/config"
	"github.com/darkyeg/spinup/internal/library"
	"github.com/darkyeg/spinup/internal/skills"
)

type libraryCmd struct {
	History libraryHistoryCmd `cmd:"" help:"List locally recorded shared instruction and SKILL.md versions."`
	Diff    libraryDiffCmd    `cmd:"" help:"Preview a document revision against its current content."`
	Restore libraryRestoreCmd `cmd:"" help:"Restore one version as a new shared library edit."`
}
type libraryHistoryCmd struct {
	Path string `arg:"" optional:"" help:"AGENTS.md or skills/<name>/SKILL.md."`
}
type libraryDiffCmd struct {
	Path     string `arg:""`
	Revision string `arg:"" help:"Content ID from library history."`
}
type libraryRestoreCmd struct {
	Path     string `arg:""`
	Revision string `arg:""`
	Expect   string `required:"" help:"Current content ID (or missing) shown by library diff."`
}

func (cliBackend) DocumentLibrary() library.Library {
	if os.Getenv("SPINUP_LIBRARY") == "" {
		if c, err := config.Load(); err == nil && c.Library != "" {
			return library.At(c.Library)
		}
	}
	return library.Here()
}

func (c libraryHistoryCmd) Run() error {
	list, err := (cliBackend{}).DocumentLibrary().Revisions(context.Background())
	if err != nil {
		return err
	}
	n := 0
	for _, v := range list {
		if c.Path == "" || c.Path == v.Path {
			fmt.Printf("%s  %s  %s\n", v.At.Local().Format("2006-01-02 15:04:05"), v.ID, v.Path)
			n++
		}
	}
	if n == 0 {
		fmt.Println("No recorded revisions.")
	}
	return nil
}
func (c libraryDiffCmd) Run() error {
	p, err := (cliBackend{}).DocumentLibrary().PreviewRevision(context.Background(), c.Path, c.Revision)
	if err != nil {
		return err
	}
	if p.Same {
		fmt.Println("This version matches the current document.")
	} else {
		fmt.Print(p.Diff)
	}
	fmt.Printf("Current version for --expect: %s\n", p.Before)
	return nil
}
func (c libraryRestoreCmd) Run() error {
	if err := (cliBackend{}).RestoreDocument(context.Background(), c.Path, c.Revision, c.Expect); err != nil {
		return err
	}
	fmt.Printf("Restored %s; it syncs to your machines as a new edit.\n", c.Path)
	return nil
}
func (b cliBackend) RestoreDocument(ctx context.Context, path, id, before string) error {
	b.CatchUpLibrary()
	lib := b.DocumentLibrary()
	repo := repoData()
	if path != library.Instructions {
		parts := strings.Split(path, "/")
		if len(parts) != 3 || parts[0] != library.SkillsDir || parts[2] != "SKILL.md" {
			return fmt.Errorf("unsupported shared document")
		}
		return skills.New(repo, lib, nil).RestoreAuthored(ctx, parts[1], id, before)
	}
	if err := lib.RestoreRevision(ctx, path, id, before); err != nil {
		return err
	}
	_, err := agentconfig.Install(agentconfig.Sources{Spinup: repo, Yours: lib}, agentconfig.HostHomes())
	if err != nil {
		return fmt.Errorf("revision restored; run spinup agents to apply it locally: %w", err)
	}
	return nil
}
