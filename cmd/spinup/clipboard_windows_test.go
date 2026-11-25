package main

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"testing"
	"time"
)

func TestWindowsClipboardCallback(t *testing.T) {
	const token = "synthetic-proof"
	for _, mode := range []string{"valid", "wrong proof", "cancel during read"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			server, client := net.Pipe()
			defer client.Close()
			done := make(chan struct{})
			go func() {
				defer close(done)
				text, ok, err := receiveClipboard(ctx, server, token)
				switch mode {
				case "valid":
					if !ok || err != nil || text != "synthetic\nβ" {
						t.Error("callback did not return synthetic text")
					}
				case "wrong proof":
					if ok || text != "" {
						t.Error("accepted unauthenticated clipboard callback")
					}
				case "cancel during read":
					if !ok || err == nil || text != "" {
						t.Error("cancelled callback returned text")
					}
				}
			}()
			proof := token
			if mode == "wrong proof" {
				proof = "synthetic-wrong"
			}
			if _, err := io.WriteString(client, proof); err != nil {
				t.Fatal(err)
			}
			answer := make([]byte, len(token))
			_, err := io.ReadFull(client, answer)
			if mode == "wrong proof" {
				if err == nil {
					t.Fatal("invalid callback received the server proof")
				}
			} else {
				if err != nil || string(answer) != token {
					t.Fatal("missing server proof")
				}
				if mode == "cancel during read" {
					cancel()
				} else if err := json.NewEncoder(client).Encode(sessionClipboard{Text: "synthetic\nβ"}); err != nil {
					t.Fatal(err)
				}
			}
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("callback did not close promptly")
			}
		})
	}
}
