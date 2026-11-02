// masterdrive is a private, opt-in, read-only storage mesh prototype.
// Peers are exposed only through an explicitly configured encrypted overlay.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/thebrazenbeard/masterdrive/internal/catalog"
	"github.com/thebrazenbeard/masterdrive/internal/server"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "masterdrive:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: masterdrive controller|add-host|remove-host|agent [options]")
	}
	switch args[0] {
	case "controller":
		flags := flag.NewFlagSet("controller", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		dataDir := flags.String("data-dir", defaultDataDir(), "private local registry directory")
		listen := flags.String("listen", "127.0.0.1:8788", "local browser listener")
		webDir := flags.String("web-dir", "web", "path to static UI files")
		if err := flags.Parse(args[1:]); err != nil { return err }
		if flags.NArg() != 0 { return errors.New("unexpected positional controller arguments") }
		if err := server.ValidateLoopbackListen(*listen); err != nil { return err }
		if info, err := os.Stat(*webDir); err != nil || !info.IsDir() {
			return fmt.Errorf("web directory missing or not a directory: %s", *webDir)
		}
		store, err := catalog.Open(*dataDir)
		if err != nil { return err }
		tokens := map[string]string{}
		if raw := os.Getenv("MASTERDRIVE_PEER_TOKENS_JSON"); raw != "" {
			if err := json.Unmarshal([]byte(raw), &tokens); err != nil {
				return fmt.Errorf("MASTERDRIVE_PEER_TOKENS_JSON: %w", err)
			}
		}
		mux := http.NewServeMux()
		session, err := server.NewSessionToken()
		if err != nil { return fmt.Errorf("generate local session: %w", err) }
		api, err := server.RequireSession(session, server.New(store, tokens, nil))
		if err != nil { return err }
		mux.Handle("/api/v1/", api)
		mux.Handle("/", http.FileServer(http.Dir(*webDir)))
		fmt.Fprintf(stdout, "MasterDrive local controller: http://%s/#session=%s\n", *listen, session)
		fmt.Fprintln(stdout, "Open this local link once; it activates a browser session without exposing peer credentials.")
		return http.ListenAndServe(*listen, server.RequireLoopbackHost(mux))
	case "add-host":
		flags := flag.NewFlagSet("add-host", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		dataDir := flags.String("data-dir", defaultDataDir(), "private local registry directory")
		name := flags.String("name", "", "human-readable device name")
		endpoint := flags.String("url", "", "HTTPS peer URL (loopback HTTP for local testing only)")
		if err := flags.Parse(args[1:]); err != nil { return err }
		if flags.NArg() != 0 { return errors.New("unexpected positional add-host arguments") }
		store, err := catalog.Open(*dataDir)
		if err != nil { return err }
		host, err := store.Upsert(*name, *endpoint)
		if err != nil { return err }
		fmt.Fprintf(stdout, "Added %s: %s\nConfigure MASTERDRIVE_PEER_TOKENS_JSON with key %q before starting controller.\n", host.Name, host.ID, host.ID)
		return nil
	case "remove-host":
		flags := flag.NewFlagSet("remove-host", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		dataDir := flags.String("data-dir", defaultDataDir(), "private local registry directory")
		id := flags.String("id", "", "registered device ID")
		if err := flags.Parse(args[1:]); err != nil { return err }
		if flags.NArg() != 0 { return errors.New("unexpected positional remove-host arguments") }
		if *id == "" { return errors.New("-id is required") }
		store, err := catalog.Open(*dataDir)
		if err != nil { return err }
		if err := store.Remove(*id); err != nil { return err }
		fmt.Fprintf(stdout, "Removed host %s\n", *id)
		return nil
	case "agent":
		return runAgent(args[1:], stdout)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func defaultDataDir() string {
	dir, err := os.UserConfigDir()
	if err != nil { return filepath.Join(".", ".masterdrive") }
	return filepath.Join(dir, "masterdrive")
}

// runAgent resides in agent.go so its CLI surface can follow the final agent
// package contract without coupling it to the controller API.

