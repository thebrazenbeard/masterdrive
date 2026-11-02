package main

import (
	"bytes"
	"net"
	"strings"
	"testing"
)

// A startup message is an operator-facing readiness signal. In particular,
// never print the controller's secret session fragment if the port is already
// owned by a different local process.
func TestControllerDoesNotPublishSessionLinkOnBindFailure(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil { t.Fatal(err) }
	defer occupied.Close()
	var output bytes.Buffer
	err = run([]string{
		"controller",
		"-listen", occupied.Addr().String(),
		"-data-dir", t.TempDir(),
		"-web-dir", t.TempDir(),
	}, &output)
	if err == nil { t.Fatal("expected occupied listener to fail") }
	if output.Len() != 0 || strings.Contains(output.String(), "#session=") {
		t.Fatalf("failed controller exposed a bogus session link: %q", output.String())
	}
}

func TestAgentDoesNotAnnounceReadyOnBindFailure(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil { t.Fatal(err) }
	defer occupied.Close()
	t.Setenv("MASTERDRIVE_AGENT_TOKEN", "sample-local-test-token-of-adequate-length-0123456789")
	var output bytes.Buffer
	err = run([]string{
		"agent",
		"-listen", occupied.Addr().String(),
		"-export", "samples=" + t.TempDir(),
	}, &output)
	if err == nil { t.Fatal("expected occupied listener to fail") }
	if output.Len() != 0 {
		t.Fatalf("failed agent claimed to be serving files: %q", output.String())
	}
}
