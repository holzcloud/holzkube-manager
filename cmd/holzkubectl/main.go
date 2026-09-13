// Command holzkubectl is a command-line client for a holzkube-manager
// instance (V2-API-01).
//
// It is a *client*. Every verdict it prints was reached by the server and
// every sentence it shows about a refusal is the server's own. PROJECT.md
// records the decision this is built under — one interface to maintain — and
// the only cut in which that and "have a CLI" are both true is this one: no
// domain logic here, ever. A second implementation of a rule is a second thing
// to keep in step, and two that disagree are worse than one of them not
// existing.
//
// It authenticates with a service-account token and nothing else. A CLI that
// took a password would be a second sign-in path holding a session, and
// sessions are for browsers: they are ambient, they are what the CSRF checks
// and the re-authentication window exist against, and none of that machinery
// means anything on a terminal. A token is put on each request deliberately,
// is per account, is rotatable, and every use of it is in the audit archive
// under that account's name.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

// version is set at build time. It is printed by `holzkubectl version` and
// nothing branches on it.
var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, os.Args[1:]); err != nil {
		if errors.Is(err, errUsage) {
			fmt.Fprint(os.Stderr, usage)
			os.Exit(2)
		}
		fmt.Fprintf(os.Stderr, "holzkubectl: %v\n", err)
		os.Exit(1)
	}
}
