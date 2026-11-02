package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"

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

const maxAgentGrantsJSONBytes = 1 << 20

// parseAgentGrantsJSON recognizes one strict, documented wire shape. It never
// includes input bytes or parser error text in its errors: this JSON contains
// bearer credentials and may be supplied by a secret-management mechanism.
func parseAgentGrantsJSON(raw string) ([]agent.AccessGrant, error) {
	if len(raw) == 0 || len(raw) > maxAgentGrantsJSONBytes {
		return nil, errors.New("MASTERDRIVE_AGENT_GRANTS_JSON must be a nonempty bounded JSON array")
	}
	var encoded []struct {
		BearerToken string   `json:"bearer_token"`
		ShareIDs    []string `json:"share_ids"`
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&encoded); err != nil {
		return nil, errors.New("MASTERDRIVE_AGENT_GRANTS_JSON is invalid JSON or has unknown fields")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("MASTERDRIVE_AGENT_GRANTS_JSON contains trailing data")
	}
	if len(encoded) == 0 || len(encoded) > 256 {
		return nil, errors.New("MASTERDRIVE_AGENT_GRANTS_JSON requires 1-256 grants")
	}
	grants := make([]agent.AccessGrant, 0, len(encoded))
	for _, item := range encoded {
		grants = append(grants, agent.AccessGrant{BearerToken: item.BearerToken, ShareIDs: item.ShareIDs})
	}
	return grants, nil
}

// configuredAgentHandler makes the legacy all-shares token and the optional
// scoped-grants configuration mutually exclusive. No credentials are placed
// on the CLI, persisted, or included in validation errors.
func configuredAgentHandler(exports []agent.Export, lookup func(string) (string, bool)) (*agent.Handler, error) {
	legacy, hasLegacy := lookup("MASTERDRIVE_AGENT_TOKEN")
	grantsJSON, hasGrants := lookup("MASTERDRIVE_AGENT_GRANTS_JSON")
	if hasLegacy == hasGrants {
		return nil, errors.New("set exactly one of MASTERDRIVE_AGENT_TOKEN or MASTERDRIVE_AGENT_GRANTS_JSON")
	}
	if hasLegacy {
		if legacy == "" {
			return nil, errors.New("MASTERDRIVE_AGENT_TOKEN must contain 32+ random characters")
		}
		return agent.NewHandler(exports, legacy)
	}
	grants, err := parseAgentGrantsJSON(grantsJSON)
	if err != nil { return nil, err }
	handler, err := agent.NewHandlerWithGrants(exports, grants)
	if err != nil {
		// The source constructor may mention a caller-provided share identifier.
		// Do not echo any value from a credential-bearing configuration here.
		return nil, errors.New("MASTERDRIVE_AGENT_GRANTS_JSON has invalid tokens or share grants")
	}
	return handler, nil
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
	handler, err := configuredAgentHandler(exports, os.LookupEnv)
	if err != nil { return err }
	defer handler.Close()
	listener, err := net.Listen("tcp", *listen)
	if err != nil { return fmt.Errorf("agent listen: %w", err) }
	defer listener.Close()
	fmt.Fprintf(stdout, "MasterDrive agent serving %d explicit read-only shares on %s\n", len(exports), *listen)
	// A private overlay reverse proxy may preserve its external Host header,
	// so the agent deliberately does not impose the local browser Host guard.
	return http.Serve(listener, handler)
}
