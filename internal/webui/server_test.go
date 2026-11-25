package webui

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/darkyeg/spinup/internal/api"
	"github.com/darkyeg/spinup/internal/config"
	"github.com/darkyeg/spinup/internal/doctor"
	"github.com/darkyeg/spinup/internal/leadership"
	"github.com/darkyeg/spinup/internal/logins"
	"github.com/darkyeg/spinup/internal/proxy"
	"github.com/darkyeg/spinup/internal/skills"
	"github.com/darkyeg/spinup/internal/t3code"
	"github.com/darkyeg/spinup/internal/tailnet"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "spillup-dashboard-test-")
	if err != nil {
		panic(err)
	}
	if err := os.Setenv("SPINUP_HOME", dir); err != nil {
		panic(err)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

type fakeBackend struct {
	mainTo string
	panels map[string]Panel
}

func (*fakeBackend) SharedQuotas(context.Context) (api.QuotaSummary, error) {
	return api.QuotaSummary{Accounts: []api.QuotaAccount{}}, nil
}
func (*fakeBackend) Facts() webuiFacts {
	return webuiFacts{Installed: true, Config: config.Defaults(), Version: "test"}
}
func (*fakeBackend) Report(context.Context) (api.Report, error) {
	return api.Report{Name: "desk", Hold: config.HoldHub, Leading: true}, nil
}
func (*fakeBackend) Tailnet(context.Context) (tailnet.Status, error) { return tailnet.Status{}, nil }
func (*fakeBackend) Doctor(context.Context) []doctor.Section         { return nil }
func (*fakeBackend) Keys() (config.Secrets, error)                   { return config.Secrets{APIKey: "k"}, nil }
func (*fakeBackend) HandOff(string) error                            { return errors.New("no") }
func (*fakeBackend) TakeOver() error                                 { return nil }
func (b *fakeBackend) MakeMain(m string) error                       { b.mainTo = m; return nil }
func (*fakeBackend) RestartProxy() error                             { return nil }
func (*fakeBackend) Skills() skills.Manager                          { return skills.Manager{} }
func (*fakeBackend) CatchUpLibrary()                                 {}
func (*fakeBackend) OwnLogins() ([]logins.Summary, error)            { return nil, nil }
func (*fakeBackend) SetOwn(context.Context, []string) error          { return nil }
func (*fakeBackend) T3DesktopLink(context.Context, string) (string, error) {
	return "t3code://pair?test=fixture", nil
}
func (*fakeBackend) T3State(context.Context) T3State                           { return T3State{} }
func (*fakeBackend) T3Phones(context.Context) ([]t3code.Phone, error)          { return nil, nil }
func (*fakeBackend) T3Pair(context.Context, string) (string, error)            { return "", nil }
func (*fakeBackend) T3Link(context.Context, string, string) error              { return nil }
func (*fakeBackend) RemoteState(context.Context) RemoteState                   { return RemoteState{} }
func (*fakeBackend) SunshinePIN(context.Context, string, string, string) error { return nil }
func (*fakeBackend) RemoteSync(context.Context) (string, error)                { return "", nil }
func (*fakeBackend) UpdateSystem() error                                       { return nil }
func (*fakeBackend) RemoteSetup() error                                        { return nil }
func (*fakeBackend) RemoteClipboard(context.Context, uint32) (ClipboardText, error) {
	return ClipboardText{}, nil
}
func (*fakeBackend) HostClipboard(context.Context, string) (ClipboardText, error) {
	return ClipboardText{}, nil
}
func (*fakeBackend) RemoteWebPort() int                             { return 1 }
func (*fakeBackend) RestartService() error                          { return nil }
func (*fakeBackend) AnswerSetup(string, bool) error                 { return nil }
func (*fakeBackend) SetHubOnly(bool) error                          { return nil }
func (*fakeBackend) ScanDevices(context.Context) ([]Device, error)  { return nil, nil }
func (*fakeBackend) T3EnableLinking() error                         { return nil }
func (*fakeBackend) T3Setup(context.Context) error                  { return nil }
func (*fakeBackend) RunFix(context.Context, string) (string, error) { return "", nil }
func (*fakeBackend) PreviewSetup(_ context.Context, name, _ string) (SetupPreview, error) {
	return SetupPreview{Name: name, Role: config.HoldNever, Changes: []SetupChange{}}, nil
}
func (*fakeBackend) StartSetup(string, string) error        { return nil }
func (*fakeBackend) SetupProgress() SetupProgress           { return SetupProgress{} }
func (*fakeBackend) SyncT3(context.Context) (string, error) { return "", nil }
func (*fakeBackend) UpdateState(context.Context) Update     { return Update{Current: "test"} }
func (*fakeBackend) Apps() []App                            { return nil }
func (*fakeBackend) RolloutPlan(context.Context) (FleetPreview, error) {
	return FleetPreview{Output: "fleet preview"}, nil
}
func (*fakeBackend) StartRollout(context.Context, string) error { return nil }
func (*fakeBackend) StartUpdate() error                         { return nil }
func (*fakeBackend) SetAutoUpdate(bool) error                   { return nil }
func (*fakeBackend) SSHState(context.Context) SSHState          { return SSHState{} }
func (*fakeBackend) SetAccess(bool, bool) error                 { return nil }
func (*fakeBackend) SSHEnable(bool) error                       { return nil }
func (*fakeBackend) SSHSync(context.Context) (string, error)    { return "", nil }
func (*fakeBackend) Remote(string) (Remote, error)              { return Remote{}, errors.New("no remote in tests") }

func (b *fakeBackend) Panel(target string) (Panel, error) {
	if p, ok := b.panels[target]; ok {
		return p, nil
	}
	return Panel{}, errors.New("no such proxy")
}
func (*fakeBackend) OwnManagement() (proxy.Management, error) { return proxy.Management{}, nil }

type webuiFacts = Facts

const testToken = "0123456789abcdef0123456789abcdef"

func do(t *testing.T, h http.Handler, method, url string, mutate func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	var body *strings.Reader
	if method == http.MethodPost {
		body = strings.NewReader(`{"machine":"Laptop"}`)
	} else {
		body = strings.NewReader("")
	}
	r := httptest.NewRequest(method, url, body)
	r.Host = "localhost:8320"
	if mutate != nil {
		mutate(r)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func signedIn(r *http.Request) {
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: testToken})
	r.Header.Set("Content-Type", "application/json")
}

func TestTheDashboardAnswersOnlyTheBrowserItSignedIn(t *testing.T) {
	b := &fakeBackend{}
	h := New(b, testToken).Handler()

	if w := do(t, h, http.MethodGet, "/api/overview", nil); w.Code != http.StatusUnauthorized {
		t.Errorf("without a session: %d, want 401", w.Code)
	}
	w := do(t, h, http.MethodGet, "/?token="+testToken, nil)
	if w.Code != http.StatusSeeOther || !strings.Contains(w.Header().Get("Set-Cookie"), sessionCookie) {
		t.Fatalf("sign-in: %d %q, want a redirect that sets the cookie", w.Code, w.Header().Get("Set-Cookie"))
	}
	if loc := w.Header().Get("Location"); strings.Contains(loc, "token") {
		t.Errorf("sign-in left the token in the address: %q", loc)
	}
	if w := do(t, h, http.MethodGet, "/?token=wrong", nil); w.Code != http.StatusUnauthorized {
		t.Errorf("a wrong token: %d, want 401", w.Code)
	}
	if w := do(t, h, http.MethodGet, "/api/overview", signedIn); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"name":"desk"`) {
		t.Errorf("overview: %d %s", w.Code, w.Body)
	}
}

func TestTheDashboardRefusesOtherSitesAndOtherNames(t *testing.T) {
	b := &fakeBackend{}
	h := New(b, testToken).Handler()

	rebound := func(r *http.Request) { signedIn(r); r.Host = "evil.example:8320" }
	if w := do(t, h, http.MethodGet, "/api/overview", rebound); w.Code != http.StatusMisdirectedRequest {
		t.Errorf("a rebound name: %d, want 421", w.Code)
	}
	crossSite := func(r *http.Request) { signedIn(r); r.Header.Set("Origin", "https://evil.example") }
	if w := do(t, h, http.MethodPost, "/api/machines/main", crossSite); w.Code != http.StatusForbidden {
		t.Errorf("a cross-site post: %d, want 403", w.Code)
	}
	form := func(r *http.Request) { signedIn(r); r.Header.Set("Content-Type", "text/plain") }
	if w := do(t, h, http.MethodPost, "/api/machines/main", form); w.Code != http.StatusForbidden {
		t.Errorf("a post that isn't JSON: %d, want 403", w.Code)
	}
	if b.mainTo != "" {
		t.Fatalf("a refused request changed the hub to %q", b.mainTo)
	}
	sameSite := func(r *http.Request) { signedIn(r); r.Header.Set("Origin", "http://localhost:8320") }
	if w := do(t, h, http.MethodPost, "/api/machines/main", sameSite); w.Code != http.StatusOK || b.mainTo != "laptop" {
		t.Errorf("make main: %d, hub %q; want 200 and laptop", w.Code, b.mainTo)
	}
}

func TestAnyPageServesTheAppSoItsRoutesSurviveAReload(t *testing.T) {
	h := New(&fakeBackend{}, testToken).Handler()
	w := do(t, h, http.MethodGet, "/skills", nil)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "<") {
		t.Fatalf("/skills: %d", w.Code)
	}
	if w := do(t, h, http.MethodGet, "/api/nope", signedIn); w.Code != http.StatusNotFound {
		t.Errorf("unknown call: %d, want 404", w.Code)
	}
}

func TestMachinesShowWhoHoldsTheAccounts(t *testing.T) {
	ts := tailnet.Status{
		Self: tailnet.Node{Name: "laptop", IP: "100.64.0.2", Online: true},
		Peers: []tailnet.Node{
			{Name: "phone", IP: "100.64.0.9", Online: true},
			{Name: "desk", IP: "100.64.0.1", Online: true},
		},
	}
	rep := &api.Report{Name: "laptop", Hold: config.HoldStandby, Leader: "desk", Peers: []leadership.Peer{
		{Name: "desk", Hold: config.HoldHub, State: leadership.Leading},
	}}
	got := machinesFrom(ts, rep)
	if len(got) != 3 || got[0].Name != "laptop" || !got[0].IsSelf || got[0].Hold != config.HoldStandby {
		t.Fatalf("this machine first, as standby: %+v", got)
	}
	desk, phone := got[1], got[2]
	if desk.Name != "desk" || desk.Hold != config.HoldHub || !desk.Leading || desk.State != "leading" {
		t.Errorf("desk: %+v", desk)
	}
	if phone.Hold != "" || phone.Leading {
		t.Errorf("a device that only uses the accounts: %+v", phone)
	}
}

func TestDashboardActionsAreRecordedWithoutRequestBodies(t *testing.T) {
	h := New(&fakeBackend{}, testToken).Handler()
	post := httptest.NewRequest(http.MethodPost, "/api/machines/main", strings.NewReader(`{"machine":"private-value"}`))
	signedIn(post)
	post.Host = "localhost:8320"
	post.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, post)
	if w.Code != http.StatusOK {
		t.Fatalf("action returned %d: %s", w.Code, w.Body.String())
	}
	log := do(t, New(&fakeBackend{}, testToken).Handler(), http.MethodGet, "/api/activity", signedIn)
	if log.Code != http.StatusOK || !strings.Contains(log.Body.String(), `"action":"machines/main"`) || strings.Contains(log.Body.String(), "private-value") {
		t.Fatalf("activity returned %d: %s", log.Code, log.Body.String())
	}
	if got := do(t, h, http.MethodGet, "/api/activity", nil).Code; got != http.StatusUnauthorized {
		t.Fatalf("unsigned activity request returned %d", got)
	}
}

func TestDashboardRejectsAnotherMutationWhileBusy(t *testing.T) {
	s := New(&fakeBackend{}, testToken)
	s.changing.Lock()
	defer s.changing.Unlock()
	w := do(t, s.Handler(), http.MethodPost, "/api/t3/sync", func(r *http.Request) {
		signedIn(r)
		r.Header.Set("Content-Type", "application/json")
	})
	if w.Code != http.StatusConflict {
		t.Fatalf("busy mutation returned %d", w.Code)
	}
}
