package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/darkyeg/spinup/internal/route"
)

// signInPaths start each family's OAuth on CLIProxyAPI's management API.
var signInPaths = map[route.Family]string{
	route.Claude: "anthropic-auth-url",
	route.GPT:    "codex-auth-url",
	route.Gemini: "antigravity-auth-url",
}

// SignIn is an OAuth sign-in a browser on this machine completes at URL.
type SignIn struct {
	URL   string `json:"url"`
	State string `json:"state"`
}

// SignInStatus is how a sign-in is going: "wait" until the browser comes back, then "ok" or "error".
type SignInStatus struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// Management calls one CLIProxyAPI's management API on 127.0.0.1.
type Management struct {
	Port int
	Key  string
}

// StartSignIn asks the proxy for an OAuth link; the proxy catches the callback itself, as long as the
// browser that opens the link runs on this machine.
func (m Management) StartSignIn(ctx context.Context, f route.Family) (SignIn, error) {
	path, ok := signInPaths[f]
	if !ok {
		return SignIn{}, fmt.Errorf("no sign-in for %q", f)
	}
	var out struct {
		SignIn
		Error string `json:"error"`
	}
	if err := m.call(ctx, http.MethodGet, path+"?is_webui=true", &out); err != nil {
		return SignIn{}, err
	}
	if out.URL == "" || out.State == "" {
		return SignIn{}, fmt.Errorf("CLIProxyAPI gave no sign-in link (%s)", out.Error)
	}
	return out.SignIn, nil
}

func (m Management) SignInStatus(ctx context.Context, state string) (SignInStatus, error) {
	var out SignInStatus
	err := m.call(ctx, http.MethodGet, "get-auth-status?state="+url.QueryEscape(state), &out)
	return out, err
}

// SignOut deletes one login file from this proxy.
func (m Management) SignOut(ctx context.Context, name string) error {
	return m.call(ctx, http.MethodDelete, "auth-files?name="+url.QueryEscape(name), nil)
}

func (m Management) call(ctx context.Context, method, path string, out any) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, fmt.Sprintf("http://127.0.0.1:%d/v0/management/%s", m.Port, path), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+m.Key)
	resp, err := (&http.Client{Transport: &http.Transport{Proxy: nil}}).Do(req)
	if err != nil {
		return fmt.Errorf("this machine's own CLIProxyAPI isn't answering: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode/100 != 2 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(body, &e)
		if e.Error == "" {
			e.Error = resp.Status
		}
		return errors.New("CLIProxyAPI: " + e.Error)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(body, out)
}
