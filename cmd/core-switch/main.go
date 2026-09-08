// core-switch atomically points a running terminal edge at a ready replacement.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tylerhardison/race-condition-kingdom/internal/terminalwire"
)

func run() error {
	target := flag.String("target", "", "new private core URL or unix:/absolute/path")
	file := flag.String("file", os.Getenv("CORE_TARGET_FILE"), "target file read by the edge")
	flag.Parse()
	token := os.Getenv("CORE_TOKEN")
	if *target == "" || *file == "" || len(token) < 32 {
		return fmt.Errorf("-target, -file and CORE_TOKEN (at least 32 bytes) are required")
	}
	base := *target
	transport := &http.Transport{}
	defer transport.CloseIdleConnections()
	if strings.HasPrefix(*target, "unix:") {
		base = "http://core"
		path := strings.TrimPrefix(*target, "unix:")
		if !filepath.IsAbs(path) {
			return fmt.Errorf("Unix socket path must be absolute")
		}
		transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", path)
		}
	}
	req, err := http.NewRequest("GET", strings.TrimRight(base, "/")+"/ready", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return fmt.Errorf("replacement is not ready: HTTP %d", response.StatusCode)
	}
	var ready terminalwire.Response
	if err = json.NewDecoder(response.Body).Decode(&ready); err != nil {
		return err
	}
	if ready.Version != terminalwire.Version || ready.CoreID == "" {
		return fmt.Errorf("replacement uses an incompatible protocol")
	}
	// Rename within the same directory; readers see either complete target.
	tmp, err := os.CreateTemp(filepath.Dir(*file), ".core-target-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err = fmt.Fprintln(tmp, *target); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = os.Rename(tmp.Name(), *file); err != nil {
		return err
	}
	fmt.Printf("Terminal edge switched to core %s at %s. Existing sockets remain open.\n", ready.CoreID, *target)
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
