package remoteweb

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestUnpackKeepsOnlyThePackageFolder(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range map[string]string{"package/web-server": "exe", "package/static/index.html": "<html>", "evil/../../x": "no"} {
		tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg})
		tw.Write([]byte(body))
	}
	tw.Close()
	gz.Close()
	files, err := unpack("x.tar.gz", buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || string(files["web-server"]) != "exe" || string(files["static/index.html"]) != "<html>" {
		t.Errorf("unpacked %v", files)
	}
}

func TestPairReadsThePinThenTheResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(UserHeader) != User || r.URL.Path != Prefix+"/api/pair" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		fmt.Fprintln(w, `{"Pin":"1234"}`)
		w.(http.Flusher).Flush()
		fmt.Fprintln(w, `{"Paired":{"host_id":1}}`)
	}))
	defer srv.Close()
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	p, _ := strconv.Atoi(port)
	pin, wait, err := Client{Port: p}.Pair(context.Background(), 1)
	if err != nil || pin != "1234" {
		t.Fatalf("pin %q %v", pin, err)
	}
	if err := wait(); err != nil {
		t.Errorf("paired: %v", err)
	}
}
