package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/darkyeg/spinup/internal/sysproc"
	"golang.org/x/sys/windows"
)

type clipboardSessionCmd struct {
	Port  uint16 `arg:""`
	Token string `arg:""`
}

type sessionClipboard struct {
	Text        string `json:"text"`
	Unavailable bool   `json:"unavailable,omitempty"`
}

func (c clipboardSessionCmd) Run() error {
	token, err := hex.DecodeString(c.Token)
	if err != nil || len(token) != 24 || c.Port == 0 {
		return errors.New("invalid clipboard session")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(int(c.Port))))
	if err != nil {
		return err
	}
	defer conn.Close()
	end, _ := ctx.Deadline()
	_ = conn.SetDeadline(end)
	if _, err = io.WriteString(conn, c.Token); err != nil {
		return err
	}
	answer := make([]byte, len(c.Token))
	if _, err := io.ReadFull(conn, answer); err != nil || subtle.ConstantTimeCompare(answer, []byte(c.Token)) != 1 {
		return errors.New("clipboard session expired")
	}
	// After the proof, the requester only reads. Closing its socket cancels a slow clipboard read.
	go func() {
		var unused [1]byte
		_, _ = conn.Read(unused[:])
		cancel()
	}()
	value, err := readClipboard(ctx)
	return json.NewEncoder(conn).Encode(sessionClipboard{Text: value, Unavailable: err != nil})
}

func desktopClipboard(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 16*time.Second)
	defer cancel()
	var session uint32
	if windows.ProcessIdToSessionId(uint32(os.Getpid()), &session) == nil && session != 0 {
		return readClipboard(ctx)
	}
	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		return "", err
	}
	defer listener.Close()
	stopAccept := context.AfterFunc(ctx, func() { _ = listener.Close() })
	defer stopAccept()
	var random [24]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	token := hex.EncodeToString(random[:])
	task := "spillup-clipboard-" + token[:16]
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	run := func(c context.Context, args ...string) error {
		cmd := exec.CommandContext(c, "schtasks.exe", args...)
		cmd.WaitDelay = time.Second
		sysproc.Hide(cmd)
		return cmd.Run()
	}
	command := fmt.Sprintf(`"%s" clipboard-session %d %s`, exe, listener.Addr().(*net.TCPAddr).Port, token)
	defer func() {
		c, stop := context.WithTimeout(context.Background(), 3*time.Second)
		defer stop()
		_ = run(c, "/Delete", "/TN", task, "/F")
	}()
	if err := run(ctx, "/Create", "/TN", task, "/TR", command, "/SC", "ONCE", "/ST", "00:00", "/IT"); err != nil {
		return "", errors.New("Windows could not reach the signed-in desktop")
	}
	if err := run(ctx, "/Run", "/TN", task); err != nil {
		return "", errors.New("Windows could not start the desktop clipboard helper")
	}
	for ctx.Err() == nil {
		conn, err := listener.Accept()
		if err != nil {
			break
		}
		value, authenticated, err := receiveClipboard(ctx, conn, token)
		if authenticated {
			return value, err
		}
	}
	return "", errors.New("the signed-in Windows desktop did not answer")
}

func receiveClipboard(ctx context.Context, conn net.Conn, token string) (string, bool, error) {
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	proof := make([]byte, len(token))
	if _, err := io.ReadFull(conn, proof); err != nil || subtle.ConstantTimeCompare(proof, []byte(token)) != 1 {
		return "", false, nil
	}
	end, _ := ctx.Deadline()
	_ = conn.SetDeadline(end)
	if _, err := io.WriteString(conn, token); err != nil {
		return "", true, err
	}
	var result sessionClipboard
	err := json.NewDecoder(bufio.NewReader(io.LimitReader(conn, 6*clipboardLimit+1024))).Decode(&result)
	if err != nil || result.Unavailable || len(result.Text) > clipboardLimit || strings.IndexByte(result.Text, 0) >= 0 {
		return "", true, errors.New("text clipboard unavailable in the signed-in Windows desktop")
	}
	return result.Text, true, nil
}
