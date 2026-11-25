package sunshine

import (
	"context"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"strings"
)

func ClipboardPeer(ctx context.Context, login Login, machine string) error {
	var clients struct {
		Clients []struct {
			Name    string
			Enabled *bool
		} `json:"named_certs"`
	}
	if err := call(ctx, login, http.MethodGet, "/api/clients/list", nil, &clients); err != nil {
		return errors.New("Sunshine could not verify paired devices")
	}
	for _, c := range clients.Clients {
		if (c.Enabled == nil || *c.Enabled) && strings.EqualFold(c.Name, "spillup on "+machine) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:47989/serverinfo", nil)
			if err != nil {
				return err
			}
			res, err := client.Do(req)
			if err != nil {
				return errors.New("Sunshine is unavailable")
			}
			defer res.Body.Close()
			var state struct {
				State string `xml:"state"`
			}
			if res.StatusCode == http.StatusOK && xml.NewDecoder(io.LimitReader(res.Body, 64<<10)).Decode(&state) == nil && state.State == "SUNSHINE_SERVER_BUSY" {
				return nil
			}
			return errors.New("start a remote session before copying")
		}
	}
	return errors.New("this device has no enabled Sunshine pairing")
}
