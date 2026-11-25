package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// askpassCmd is sudo's password helper while the dashboard runs setup: it asks the dashboard, which asks
// you, and prints your answer for sudo. Nothing keeps the password.
type askpassCmd struct {
	Prompt string `arg:"" optional:""`
}

func (c askpassCmd) Run() error {
	addr, token := os.Getenv("SPINUP_ASKPASS_ADDR"), os.Getenv("SPINUP_ASKPASS_TOKEN")
	if addr == "" || token == "" {
		return errors.New("spinup askpass runs only under the dashboard's setup")
	}
	req, err := http.NewRequest(http.MethodPost, "http://"+addr+"/ask", strings.NewReader(c.Prompt))
	if err != nil {
		return err
	}
	req.Header.Set("X-Spinup-Askpass", token)
	client := *directMachineClient
	client.Timeout = 10 * time.Minute
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New("no password given")
	}
	answer, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return err
	}
	fmt.Println(string(answer))
	return nil
}
