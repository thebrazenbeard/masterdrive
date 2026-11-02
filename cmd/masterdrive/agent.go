package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"os"

	"github.com/thebrazenbeard/masterdrive/internal/agent"
	"github.com/thebrazenbeard/masterdrive/internal/server"
)

type exportFlags []agent.Export

func (e *exportFlags) String() string { return fmt.Sprintf("%d exports", len(*e)) }

func (e *exportFlags) Set(value string) error {
	id, directory, found := strings.Cut(value, "=")
	if !found || id == "" || directory == "" {
		return errors.New("export must be ID=/absolute/directory")
	}
	*e = append(*e, agent.Export{ID: id, Name: id, Path: directory})
	return nil
}

func runAgent(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("agent", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	listen := flags.String("listen", "127.0.0.1:8789", "local agent listener")
	var exports exportFlags
	flags.Var(&exports, "export", "explicit read-only share ID=/absolute/directory (repeatable)")
	if err := flags.Parse(args); err != nil { return err }
	if flags.NArg() != 0 { return errors.New("unexpected positional agent arguments") }
	if len(exports) == 0 { return errors.New("at least one -export is required") }
	if err := server.ValidateLoopbackListen(*listen); err != nil { return err }
	secret := os.Getenv("MASTERDRIVE_AGENT_TOKEN")
	if secret == "" { return errors.New("MASTERDRIVE_AGENT_TOKEN must be set (32+ random characters)") }
	handler, err := agent.NewHandler(exports, secret)
	if err != nil { return err }
	defer handler.Close()
	listener, err := net.Listen("tcp", *listen)
	if err != nil { return fmt.Errorf("agent listen: %w", err) }
	defer listener.Close()
	fmt.Fprintf(stdout, "MasterDrive agent serving %d explicit read-only shares on %s\n", len(exports), *listen)
	// The agent requires a bearer token for every request. A private overlay
	// reverse proxy may preserve its external Host header, so it deliberately
	// does not impose the browser controller's loopback Host restriction.
	return http.Serve(listener, handler)
}
