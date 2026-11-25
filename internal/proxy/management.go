package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// healthy reports whether a CLIProxyAPI answers at base.
func healthy(ctx context.Context, base string) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, base+"/", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return strings.Contains(string(body), "CLI Proxy API")
}

// LoginState is one login as the running proxy sees it.
type LoginState struct {
	Name          string `json:"name"`
	Status        string `json:"status"`
	StatusMessage string `json:"status_message"`
	Unavailable   bool   `json:"unavailable"`
	Disabled      bool   `json:"disabled"`
}

// Refused reports whether the provider refused the login's token, as opposed to a rate limit.
func (s LoginState) Refused() bool {
	msg := strings.ToLower(s.StatusMessage)
	refused := strings.Contains(msg, "invalid_grant") || strings.Contains(msg, "refresh") ||
		strings.Contains(msg, "401") || strings.Contains(msg, "unauthorized")
	return !s.Disabled && refused && (s.Unavailable || strings.EqualFold(s.Status, "error"))
}

func LoginStates(ctx context.Context, port int, managementPassword string) ([]LoginState, error) {
	var out struct {
		Files []LoginState `json:"files"`
	}
	err := readAuthFiles(ctx, port, managementPassword, &out)
	return out.Files, err
}

var managementReader = &http.Client{
	Transport:     &http.Transport{Proxy: nil, IdleConnTimeout: 30 * time.Second},
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

func readAuthFiles(ctx context.Context, port int, password string, out any) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return readManagement(ctx, port, password, http.MethodGet, "auth-files", nil, 8<<20, out)
}

func readManagement(ctx context.Context, port int, password, method, path string, payload io.Reader, limit int64, out any) error {
	req, err := http.NewRequestWithContext(ctx, method,
		fmt.Sprintf("http://127.0.0.1:%d/v0/management/%s", port, path), payload)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+password)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := managementReader.Do(req)
	if err != nil {
		return errors.New("CLIProxyAPI did not answer on the holder")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("CLIProxyAPI management returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil || int64(len(body)) > limit || json.Unmarshal(body, out) != nil {
		return errors.New("CLIProxyAPI returned an incomplete management response")
	}
	return nil
}

// AnswersOn reports whether a CLIProxyAPI answers on 127.0.0.1:port.
func AnswersOn(ctx context.Context, port int) bool {
	return healthy(ctx, fmt.Sprintf("http://127.0.0.1:%d", port))
}
