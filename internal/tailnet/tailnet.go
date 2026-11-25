// Package tailnet reads the tailnet from the tailscale CLI: which machines exist, their IPs, and
// whether Tailscale's control server sees them online.
package tailnet

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/darkyeg/spinup/internal/sysproc"
)

// Node is one machine on the tailnet.
type Node struct {
	ID      string // Tailscale stable node ID, when available.
	Name    string // MagicDNS short name, e.g. "office-pc"
	DNSName string // full MagicDNS name, e.g. "office-pc.tail1234.ts.net"
	IP      string // Tailscale IPv4
	IPv6    string // Tailscale IPv6
	OS      string
	// Online is the control server's view. Unlike "I can't reach it", this stays true when only
	// the path between two machines is broken, so it is the referee for failover. For this machine
	// it means "connected to the control server".
	Online bool
	// LastSeen is when the control server last saw it (zero while online).
	LastSeen time.Time
	// KeyExpiry is when the machine's login lapses; zero when the key never expires.
	KeyExpiry time.Time
	// Expired means the login has already lapsed, so the machine is off the tailnet until someone logs in on it.
	Expired bool
	IsSelf  bool
	// UserID is the Tailscale account the device is signed in with; a tagged device belongs to no one.
	UserID int64
	Tagged bool
}

func (n Node) Mobile() bool {
	return strings.EqualFold(n.OS, "android") || strings.EqualFold(n.OS, "ios") || strings.EqualFold(n.OS, "ipados")
}

// HTTPURL gives the node's MagicDNS URL, falling back to its Tailscale address.
func (s Status) HTTPURL(n Node, port int) string {
	if n.IP == "" || port == 0 {
		return ""
	}
	host := n.IP
	if s.MagicDNS && n.DNSName != "" {
		host = strings.TrimSuffix(n.DNSName, ".")
	}
	return "http://" + net.JoinHostPort(host, strconv.Itoa(port))
}

// SameOwner reports whether a peer is signed in with this machine's own Tailscale account, untagged: one
// of your own devices, which Tailscale vouches for by its address.
func (s Status) SameOwner(peer Node) bool {
	return peer.UserID != 0 && peer.UserID == s.Self.UserID && !peer.Tagged && !s.Self.Tagged
}

// PeerByIP finds the peer with a Tailscale address.
func (s Status) PeerByIP(ip string) (Node, bool) {
	for _, p := range s.Peers {
		if p.IP == ip {
			return p, true
		}
	}
	return Node{}, false
}

// Status is the tailnet as this machine sees it.
type Status struct {
	Running      bool
	BackendState string
	Self         Node
	Peers        []Node
	// CertDomains is empty while HTTPS certificates are off for the tailnet.
	CertDomains []string
	MagicDNS    bool
}

// Peer returns the peer called name.
func (s Status) Peer(name string) (Node, bool) {
	for _, p := range s.Peers {
		if p.Name == name {
			return p, true
		}
	}
	return Node{}, false
}

// Source gives the current tailnet status. Tests use a fake.
type Source interface {
	Status(ctx context.Context) (Status, error)
}

// CLI runs `tailscale status --json`.
type CLI struct{ Bin string }

// Find locates the tailscale CLI.
func Find() string {
	if p, err := exec.LookPath("tailscale"); err == nil {
		return p
	}
	for _, p := range []string{
		`C:\Program Files\Tailscale\tailscale.exe`,
		"/opt/homebrew/bin/tailscale", "/usr/local/bin/tailscale",
		"/Applications/Tailscale.app/Contents/MacOS/Tailscale",
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// errNoTailscale means the CLI isn't installed.
var errNoTailscale = errors.New("tailscale CLI not found")

type rawNode struct {
	ID           string
	HostName     string
	DNSName      string
	OS           string
	TailscaleIPs []string
	Online       bool
	LastSeen     time.Time
	KeyExpiry    time.Time
	Expired      bool
	UserID       int64
	Tags         []string
}

func (c CLI) bin() (string, error) {
	if c.Bin != "" {
		return c.Bin, nil
	}
	if bin := Find(); bin != "" {
		return bin, nil
	}
	return "", errNoTailscale
}

// statusWithin bounds one `tailscale status` run; a leader's fence budget counts on it.
const statusWithin = 10 * time.Second

// Status implements Source. Callers that ask at the same time share one CLI run, and a good reading answers for
// reuseWithin, so the service's leadership tick,
// the dashboard and background jobs don't pile up processes on a loaded machine until the tick times out.
func (c CLI) Status(ctx context.Context) (Status, error) {
	bin, err := c.bin()
	if err != nil {
		return Status{}, err
	}
	if st, ok := statusRuns.recent(bin); ok {
		return st, nil
	}
	f, first := statusRuns.join(bin)
	if first {
		f.st, f.err = c.read(context.WithoutCancel(ctx), bin)
		statusRuns.finish(bin, f)
	}
	select {
	case <-f.done:
		return f.st, f.err
	case <-ctx.Done():
		return Status{}, ctx.Err()
	}
}

// read retries a quick failure once: a busy CLI or local API is not a lost tailnet. A timeout is not retried.
func (c CLI) read(ctx context.Context, bin string) (Status, error) {
	st, err := c.statusOnce(ctx, bin)
	var slow errStatusTimedOut
	if err == nil || errors.As(err, &slow) || ctx.Err() != nil {
		return st, err
	}
	return c.statusOnce(ctx, bin)
}

func (c CLI) statusOnce(ctx context.Context, bin string) (Status, error) {
	ctx, cancel := context.WithTimeout(ctx, statusWithin)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "status", "--json")
	cmd.WaitDelay = time.Second
	sysproc.Hide(cmd)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil && len(out) == 0 {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return Status{}, errStatusTimedOut{}
		}
		return Status{}, statusFailed(err, stderr.String())
	}
	return parse(out)
}

// errStatusTimedOut means the CLI didn't answer within statusWithin.
type errStatusTimedOut struct{}

func (errStatusTimedOut) Error() string {
	return "tailscale status didn't answer within " + statusWithin.String()
}

// statusFailed names the CLI's own complaint, so a log says why instead of only "exit status 1".
func statusFailed(err error, stderr string) error {
	first, _, _ := strings.Cut(strings.TrimSpace(stderr), "\n")
	if first = strings.TrimSpace(first); first == "" {
		return err
	}
	return errors.New(err.Error() + ": " + first)
}

// parse decodes `tailscale status --json`.
func parse(data []byte) (Status, error) {
	var raw struct {
		BackendState   string
		Self           rawNode
		Peer           map[string]rawNode
		CertDomains    []string
		CurrentTailnet struct{ MagicDNSEnabled bool }
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return Status{}, err
	}
	st := Status{
		Running:      raw.BackendState == "Running",
		BackendState: raw.BackendState,
		Self:         convert(raw.Self),
		CertDomains:  raw.CertDomains,
		MagicDNS:     raw.CurrentTailnet.MagicDNSEnabled,
	}
	st.Self.IsSelf = true
	st.Self.Online = st.Running && raw.Self.Online
	for _, p := range raw.Peer {
		if n := convert(p); n.IP != "" {
			st.Peers = append(st.Peers, n)
		}
	}
	return st, nil
}

func convert(r rawNode) Node {
	n := Node{
		ID:      r.ID,
		Name:    shortName(r.DNSName, r.HostName),
		DNSName: strings.TrimSuffix(strings.ToLower(r.DNSName), "."),
		OS:      r.OS,
		Online:  r.Online,
		Expired: r.Expired,
		UserID:  r.UserID,
		Tagged:  len(r.Tags) > 0,
	}
	if r.LastSeen.Year() > 1 {
		n.LastSeen = r.LastSeen
	}
	if r.KeyExpiry.Year() > 1 {
		n.KeyExpiry = r.KeyExpiry
	}
	for _, raw := range r.TailscaleIPs {
		ip, err := netip.ParseAddr(raw)
		if err != nil {
			continue
		}
		if ip.Is4() && n.IP == "" {
			n.IP = ip.String()
		}
		if ip.Is6() && n.IPv6 == "" {
			n.IPv6 = ip.String()
		}
	}
	return n
}

// shortName is the MagicDNS short name ("office-pc"), or the OS hostname's.
func shortName(dnsName, hostName string) string {
	name := dnsName
	if name == "" {
		name = hostName
	}
	name, _, _ = strings.Cut(name, ".")
	return strings.ToLower(name)
}
