package webui

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/darkyeg/spinup/internal/skills"
)

func (s *Server) serveSkills(from reach) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		view, err := skillsView(s.b.Skills())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		view.SharingAvailable = from == browser
		if from != browser {
			hidden := map[string]bool{}
			view.Shared = slices.DeleteFunc(view.Shared, func(v SharedSkill) bool {
				if v.Private {
					hidden[v.Name] = true
				}
				return v.Private
			})
			view.Clashes = slices.DeleteFunc(view.Clashes, func(c Clash) bool { return slices.ContainsFunc(c.Folders, func(s string) bool { return hidden[s] }) })
			chars := 0
			for _, v := range view.Shared {
				if v.Mode == "auto" && v.Installed {
					chars += v.DescriptionChars
				}
			}
			view.TokenEstimate = chars / 4
		}
		writeJSON(w, view)
	}
}

func skillsView(m skills.Manager) (Skills, error) {
	status, err := m.Status()
	if err != nil {
		return Skills{}, err
	}
	entries, err := m.Inventory()
	if err != nil {
		return Skills{}, err
	}
	parked, err := m.Trash()
	if err != nil {
		return Skills{}, err
	}
	view := Skills{
		Library: status.Library, TokenEstimate: skills.TokenEstimate(status.Skills),
		Shared: []SharedSkill{}, OnlyHere: []LocalSkill{}, Removed: []Removed{},
	}
	agents := map[string][]string{}
	for _, e := range entries {
		switch e.State {
		case skills.Shared:
			agents[e.Name] = e.Agents
		case skills.OnlyHere, skills.AgentOnly:
			view.OnlyHere = append(view.OnlyHere, LocalSkill{
				Name: e.Name, AgentOwn: e.State == skills.AgentOnly, Agents: orEmpty(e.Agents), Path: e.Path,
			})
		}
	}
	for _, sk := range status.Skills {
		view.Shared = append(view.Shared, SharedSkill{
			Private: sk.Private, SharedAvailable: sk.SharedAvailable, Path: sk.Path, Name: sk.Name, Source: sk.Source, Mode: sk.Mode.String(), Installed: sk.Installed,
			DescriptionChars: sk.DescriptionChars, Agents: orEmpty(agents[sk.Name]),
		})
	}
	for _, p := range parked {
		view.Removed = append(view.Removed, Removed{Name: p.Name, Was: p.Was, Removed: p.Removed})
	}
	for _, c := range status.Clashes {
		view.Clashes = append(view.Clashes, Clash{Name: c.Name, Folders: c.Folders})
	}
	return view, nil
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// editSkills catches up with your other machines before an edit, so the change builds on their newest library.
func (s *Server) editSkills(edit func(skills.Manager) error) error {
	s.b.CatchUpLibrary()
	return edit(s.b.Skills())
}

func (s *Server) serveSkillMode(from reach) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req modeRequest
		if !decodeBody(w, r, &req) || !named(w, req.Names) {
			return
		}
		mode := skills.Auto
		if req.Manual {
			mode = skills.Manual
		}
		err := s.editSkills(func(m skills.Manager) error { return m.SetModeScoped(req.Names, mode, from == browser) })
		answer(w, err, fmt.Sprintf("%s %s %s now. Restart Claude Code and Codex to pick it up.", list(req.Names), isAre(req.Names), mode))
	}
}

func (s *Server) serveSkillRemove(w http.ResponseWriter, r *http.Request) {
	var req skillsRequest
	if !decodeBody(w, r, &req) || !named(w, req.Names) {
		return
	}
	err := s.editSkills(func(m skills.Manager) error {
		_, err := m.Remove(r.Context(), req.Names)
		return err
	})
	answer(w, err, fmt.Sprintf("Removed %s from every machine. Nothing is deleted: find it under Removed.", list(req.Names)))
}

func (s *Server) serveSkillRestore(w http.ResponseWriter, r *http.Request) {
	var req nameRequest
	if !decodeBody(w, r, &req) || !named(w, []string{req.Name}) {
		return
	}
	var added skills.Added
	err := s.editSkills(func(m skills.Manager) (err error) {
		added, err = m.Restore(r.Context(), req.Name)
		return err
	})
	answer(w, err, added.Name+" is back in your library, on every machine.")
}

func (s *Server) serveSkillAdopt(w http.ResponseWriter, r *http.Request) {
	var req skillsRequest
	if !decodeBody(w, r, &req) || !named(w, req.Names) {
		return
	}
	err := s.editSkills(func(m skills.Manager) error {
		_, err := m.Adopt(r.Context(), req.Names)
		return err
	})
	answer(w, err, fmt.Sprintf("%s %s in your library now and reach every machine.", list(req.Names), isAre(req.Names)))
}

func (s *Server) serveSkillAdd(from reach) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req addRequest
		if !decodeBody(w, r, &req) || !named(w, req.Names) {
			return
		}
		if req.Private && from != browser {
			writeError(w, http.StatusForbidden, "Open this machine’s local dashboard to add a private skill.")
			return
		}
		repo := strings.TrimSpace(req.Repo)
		if strings.Count(repo, "/") != 1 {
			writeError(w, http.StatusBadRequest, "name the GitHub repo as owner/repo, e.g. anthropics/skills")
			return
		}
		mode := skills.Auto
		if req.Manual {
			mode = skills.Manual
		}
		err := s.editSkills(func(m skills.Manager) error {
			if req.Private {
				_, err := m.AddPrivate(r.Context(), repo, req.Names, mode)
				return err
			}
			_, err := m.Add(r.Context(), repo, req.Names, mode)
			return err
		})
		answer(w, err, fmt.Sprintf("Added %s from %s. Restart Claude Code and Codex to load %s.", list(req.Names), repo, itThem(req.Names)))
	}
}

func (s *Server) serveSkillSync(w http.ResponseWriter, r *http.Request) {
	var synced skills.Synced
	err := s.editSkills(func(m skills.Manager) (err error) {
		synced, err = m.Sync(context.WithoutCancel(r.Context()))
		return err
	})
	msg := fmt.Sprintf("%d skills installed for %s.", synced.Count, strings.Join(synced.Agents, ", "))
	if len(synced.Parked) > 0 {
		msg += fmt.Sprintf(" Parked %s: kept, not deleted.", list(synced.Parked))
	}
	answer(w, err, msg)
}

func named(w http.ResponseWriter, names []string) bool {
	for _, n := range names {
		if strings.TrimSpace(n) == "" {
			writeError(w, http.StatusBadRequest, "a skill name is empty")
			return false
		}
	}
	if len(names) == 0 {
		writeError(w, http.StatusBadRequest, "pick at least one skill")
		return false
	}
	return true
}

func list(names []string) string {
	if len(names) <= 3 {
		return strings.Join(names, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(names[:2], ", "), len(names)-2)
}

func isAre(names []string) string {
	if len(names) == 1 {
		return "is"
	}
	return "are"
}

func itThem(names []string) string {
	if len(names) == 1 {
		return "it"
	}
	return "them"
}

func (s *Server) serveSkillSharing(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name   string `json:"name"`
		Action string `json:"action"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	err := s.editSkills(func(m skills.Manager) error { _, err := m.Sharing(r.Context(), req.Name, req.Action); return err })
	messages := map[string]string{"private": "Changes to this skill now stay on this machine.", "resume": "Private copy archived. This machine now follows the shared version.", "publish": "This working copy is now shared with the fleet."}
	answer(w, err, messages[req.Action])
}
