package webui

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/darkyeg/spinup/internal/api"
	"github.com/darkyeg/spinup/internal/doctor"
	"github.com/darkyeg/spinup/internal/tailnet"
)

func (s *Server) serveOverview(w http.ResponseWriter, r *http.Request) {
	f := s.b.Facts()
	o := Overview{
		Installed: f.Installed, Hold: f.Config.Hold, OS: f.OS, Version: f.Version, CanManage: f.CanManage,
		Settings: Settings{
			Port: f.Config.Port, ProxyPort: f.Config.ProxyPort, FailoverAfterSeconds: f.Config.FailoverAfterSeconds,
			AutoFailback: f.Config.AutoFailback, Library: f.Config.Library,
			DeviceAccess: f.Config.DeviceAccess, Terminals: f.Config.Terminals,
		},
	}
	if f.Installed {
		if rep, err := s.b.Report(r.Context()); err == nil {
			o.Service, o.Name, o.Hold = &rep, rep.Name, rep.Hold
		} else {
			o.ServiceProblem = err.Error()
		}
	}
	if st, ok := s.tailnet.get(r.Context(), s.b.Tailnet); ok {
		if f.Config.DeviceAccess {
			o.Settings.DeviceURL = st.HTTPURL(st.Self, f.Config.UIPort)
		}
		if o.Hold.CanHold() {
			o.RouterURL = st.HTTPURL(st.Self, f.Config.Port)
			if o.RouterURL != "" {
				o.DashboardURL = o.RouterURL + "/management.html"
			}
		} else if o.Service != nil {
			if leader, ok := st.Peer(o.Service.Leader); ok {
				o.RouterURL = st.HTTPURL(leader, f.Config.Port)
			}
		}
	}
	writeJSON(w, o)
}

func (s *Server) serveMachines(w http.ResponseWriter, r *http.Request) {
	ts, err := s.b.Tailnet(r.Context())
	var out Machines
	if err != nil {
		out.Problem = err.Error()
	}
	var rep *api.Report
	if s.b.Facts().Installed {
		if got, err := s.b.Report(r.Context()); err == nil {
			rep = &got
		}
	}
	out.Machines = machinesFrom(ts, rep)
	writeJSON(w, out)
}

func (s *Server) serveManagedMachines(w http.ResponseWriter, r *http.Request) {
	machines, err := s.b.ManagedMachines(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "Couldn't verify which machines are running Spillup.")
		return
	}
	if machines == nil {
		machines = []ManagedMachine{}
	}
	writeJSON(w, struct {
		Machines []ManagedMachine `json:"machines"`
	}{machines})
}

// machinesFrom is every tailnet device, this one first, with what the accounts service knows of each.
func machinesFrom(ts tailnet.Status, rep *api.Report) []Machine {
	machines := []Machine{}
	for _, n := range ts.Machines() {
		m := Machine{
			Name: n.Name, DNSName: n.DNSName, IP: n.IP, IPv6: n.IPv6, OS: n.OS, Online: n.Online, Expired: n.Expired, IsSelf: n.IsSelf,
			Yours: ts.SameOwner(n), LastSeen: timeOrNil(n.LastSeen), KeyExpiry: timeOrNil(n.KeyExpiry),
		}
		if n.Name == ts.Self.Name {
			m.IsSelf = true
		}
		if rep != nil {
			describeHolder(&m, *rep)
		}
		machines = append(machines, m)
	}
	return machines
}

func describeHolder(m *Machine, rep api.Report) {
	if m.IsSelf {
		m.Hold, m.Leading = rep.Hold, rep.Leading
		switch {
		case rep.Leading:
			m.State = "leading"
		case rep.Starting:
			m.State = "starting"
		}
		return
	}
	for _, p := range rep.Peers {
		if strings.EqualFold(p.Name, m.Name) {
			m.Hold, m.State = p.Hold, p.State.String()
		}
	}
	if rep.Leader != "" && strings.EqualFold(rep.Leader, m.Name) {
		m.Leading = true
	}
}

func timeOrNil(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func (s *Server) serveDoctor(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, DoctorReport(s.b.Doctor(r.Context())))
}

// DoctorReport formats the diagnostic checks for the dashboard and CLI.
func DoctorReport(sections []doctor.Section) Doctor {
	out := Doctor{Sections: []Section{}, Failures: doctor.Failures(sections)}
	levels := map[doctor.Level]string{doctor.OK: "ok", doctor.Warn: "warn", doctor.Fail: "fail"}
	for _, sec := range sections {
		if len(sec.Checks) == 0 {
			continue
		}
		section := Section{Title: sec.Title}
		for _, c := range sec.Checks {
			section.Checks = append(section.Checks, Check{Level: levels[c.Level], Label: c.Label, Fix: c.Fix})
		}
		out.Sections = append(out.Sections, section)
	}
	return out
}

func (s *Server) serveKeys(w http.ResponseWriter, _ *http.Request) {
	secrets, err := s.b.Keys()
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, Keys{APIKey: secrets.APIKey, ManagementPassword: secrets.ManagementPassword})
}

func (s *Server) serveHandoff(w http.ResponseWriter, r *http.Request) {
	var req handoffRequest
	if !decodeBody(w, r, &req) {
		return
	}
	to := strings.ToLower(strings.TrimSpace(req.To))
	if to == "" {
		writeError(w, http.StatusBadRequest, "name the machine that takes the accounts")
		return
	}
	answer(w, s.b.HandOff(to), to+" holds the accounts.")
}

func (s *Server) serveTakeover(w http.ResponseWriter, _ *http.Request) {
	answer(w, s.b.TakeOver(), "This machine takes the accounts within a few seconds, unless another machine holds them.")
}

func (s *Server) serveRestart(w http.ResponseWriter, _ *http.Request) {
	answer(w, s.b.RestartProxy(), "CLIProxyAPI restarted; the accounts stayed here.")
}

func (s *Server) serveMain(w http.ResponseWriter, r *http.Request) {
	var req mainRequest
	if !decodeBody(w, r, &req) {
		return
	}
	name := strings.ToLower(strings.TrimSpace(req.Machine))
	if name == "" {
		writeError(w, http.StatusBadRequest, "name the machine that should be the hub")
		return
	}
	answer(w, s.b.MakeMain(name), fmt.Sprintf("%s is the hub now. The accounts move to it between requests.", name))
}
