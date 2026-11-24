package doctor

import (
	"fmt"
	"strings"
)

// T3Facts are what the doctor knows about T3 Code here.
type T3Facts struct {
	Installed, App, ServiceAtBoot, LinkingOn, NetworkAccess bool
	Linked                                                  int
	Problem                                                 string
}

// T3 checks that T3 Code runs at boot and is linked to your other machines.
func T3(f T3Facts) Section {
	s := Section{Title: "T3 Code"}
	if !f.Installed {
		why := "T3 Code isn't installed here"
		if f.App {
			why = "T3 Code's app is here, but not the command line spillup runs it with"
		}
		return Section{"T3 Code", []Check{fail(why, "spinup t3 setup")}}
	}
	s.Checks = append(s.Checks, ok("installed"))
	if f.ServiceAtBoot {
		s.Checks = append(s.Checks, ok("its background service starts at boot"))
	} else {
		s.Checks = append(s.Checks, fail("its background service isn't set to start at boot", "spinup t3 setup"))
	}
	if !f.NetworkAccess {
		s.Checks = append(s.Checks, fail("the T3 Code app answers only this machine, so other machines can't link to it",
			"spinup t3 setup, then restart T3 Code"))
	}
	if f.LinkingOn {
		s.Checks = append(s.Checks, ok("linked environments are on"))
	} else {
		s.Checks = append(s.Checks, fail("linked environments are off, so agents can't hand work to other machines", "spinup t3 setup"))
	}
	switch {
	case f.Problem != "":
		s.Checks = append(s.Checks, warn("couldn't read its links: "+f.Problem, "spinup t3 setup"))
	case f.Linked == 0:
		s.Checks = append(s.Checks, warn("not linked to any other machine yet", "spinup t3 link"))
	default:
		s.Checks = append(s.Checks, ok("linked to "+plural(f.Linked, "1 machine", fmt.Sprintf("%d machines", f.Linked))))
	}
	return s
}

// ProxyFacts are what the doctor knows about this machine's CLIProxyAPIs.
type ProxyFacts struct {
	// ForeignOnPort: something that isn't spillup answers on spillup's port, such as an old CLIProxyAPI.
	ForeignOnPort bool
	Port          int
	Leading       bool
	Running       bool
	// KeyProblem is why the shared CLIProxyAPI refused spillup's key, when it did.
	KeyProblem   string
	OwnProviders []string
	OwnRunning   bool
}

// Proxy checks the CLIProxyAPIs spillup runs here.
func Proxy(f ProxyFacts) Section {
	s := Section{Title: "CLIProxyAPI"}
	if f.ForeignOnPort {
		s.Checks = append(s.Checks, fail(fmt.Sprintf("another program answers on port %d, where spillup's router belongs — likely an old CLIProxyAPI", f.Port),
			"stop that program, then spinup setup"))
	}
	if f.Leading {
		switch {
		case !f.Running:
			s.Checks = append(s.Checks, fail("this machine holds the accounts, but its CLIProxyAPI isn't running", "spinup restart"))
		case f.KeyProblem != "":
			s.Checks = append(s.Checks, fail("CLIProxyAPI refused spillup's key: "+f.KeyProblem, "spinup setup"))
		default:
			s.Checks = append(s.Checks, ok("the shared accounts' CLIProxyAPI runs here and accepts spillup's key"))
		}
	}
	if len(f.OwnProviders) > 0 {
		names := strings.Join(f.OwnProviders, ", ")
		if f.OwnRunning {
			s.Checks = append(s.Checks, ok("this machine's own sign-ins run ("+names+")"))
		} else {
			s.Checks = append(s.Checks, fail("this machine signs in to "+names+" by itself, but that CLIProxyAPI isn't running", "spinup restart"))
		}
	}
	if len(s.Checks) == 0 {
		s.Checks = append(s.Checks, ok("nothing runs here: this machine uses the holder's CLIProxyAPI"))
	}
	return s
}
