package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/host/hostaction"
	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/jobs"
	"github.com/holzcloud/holzkube-manager/internal/model"
)

// The machine holzkube-manager runs on (HOST-01, Phase 11).
//
// One route, and each of its settings is a decision rather than a default.
//
// **It requires a session and a reader.** The answer names the hostname, the
// board and the kernel of the machine that holds every cluster's secrets. A
// reader already sees the version and the whole cluster topology, so a reader
// may see this too; somebody who has not signed in may not.
//
// **It carries no audit Action.** It is a read that changes nothing, and an open
// page asks for it every three seconds; recording it would fill an archive D-16
// keeps for ever with the fact that somebody had a tab open. The same reasoning
// as /metrics.
//
// **It is not a wall-link route.** The wall's host tile is Phase 12, and a
// credential for a screen opens only what that screen shows.
//
// It reaches no node and no upstream service: everything it answers is read
// from this process's own namespace, through a handful of files and four
// syscalls (internal/host). That is why it is in budget_test.go's list of
// routes that reach nothing.
//
// # The host actions (HACT-01..08, Phase 13)
//
// Six more routes: POST /api/v1/host/confirm, and one
// POST /api/v1/host/actions/<action> for each of reboot, poweroff,
// restart-service, update and check-update. None of them does what it names. The daemon
// runs unprivileged and stays hardened; an action route places a one-line
// order in the data directory (internal/host/hostaction), and a root-owned
// helper, deploy/holzkube-manager-host.sh, carries it out. Nothing in this file
// starts a process.
//
// **The action routes are destructive (D-06, D-07).** Four restart or switch
// off the machine every cluster's secrets live on, or the service itself; the
// fifth, check-update, changes nothing but has root reach the network with the
// release token on the page's behalf. So each one requires a session, the
// operator role and an open sudo window, and each carries its own audit
// Action, host.<action>, written before the handler runs. The body is only the confirmation token, which never belongs in
// an archive kept for ever, so the allowlist records no parameter of them.
//
// **The confirm route is operator and not destructive**, like the node one: it
// changes nothing, it issues a token -- and only to somebody who typed the
// hostname (D-08). Every host action requires typing (D-09, HACT-05), unlike
// a node reboot: there is one host, and it is the machine this page runs on.
//
// **The token is bound to the host.** The confirm route issues it for
// {Action: host.<action>, Machine: hostIntentTarget}, and the action route
// rebuilds exactly that intent from its own path, never from the token. A
// token for a node reboot therefore never opens the host reboot, and the other
// way round.
//
// **And to the session, and it opens one order (WR-05).** D-09 asks for the
// hostname to be typed every time, and a token good for ten minutes would
// quietly undo that: one confirmation, replayed by a stale tab, a script or
// another operator holding it, would place a second reboot without anybody
// typing again. So the intent also names the session that typed (a digest of
// its id, never the id itself); the confirm route issues it with
// Confirmer.IssueOnce, and the action route checks it with CheckOnce, which
// spends it. The sudo prompt a 428 opens does not spend it: the sudo gate
// answers before the handler, and the replay after the password finds the
// token unspent.
//
// They too reach nothing upstream: they write one file in the data directory,
// and budget_test.go lists them for that reason.
//
// **Refused before anything else when nothing can carry them out (D-12,
// D-14).** Both the confirm route and every action route ask, right after the
// nil guards, whether the daemon runs in a container (409
// conflict.host-in-container) and whether the helper is installed completely
// (409 conflict.host-helper-missing) -- in that order, because in a container
// there is no helper to install and the install commands would be the wrong
// advice. The action route asks before Confirmer.Check and before Place: a
// missing helper never gets as far as the order file, so an order nobody would
// pick up is not placed in the first place (the Box's 10-s withdrawal is the
// second net, for a helper that is installed but not running). The confirm
// route asks too, so the dialog is refused before the operator has typed for
// nothing. Both questions are the ones GET /api/v1/host answers in
// actions.available, so the page and the routes cannot disagree.
//
// The check alone is asked a third question, after those two: whether the
// installed helper is new enough for it (409 conflict.host-helper-outdated).
// A helper installed before check-update existed carries out the four older
// orders and refuses the fifth; hostaction.Outdated reads that from the
// installed script's marker line and the check unit's file, and it is what
// GET /api/v1/host answers in actions.outdated. The action route asks it
// before the body and the token, the confirm route once it knows the action
// is the check and before the hostname is compared -- so no token is issued
// for a check the helper would refuse, and no order is placed. The helper's own
// refusal of every word it does not know stays the lock; this keeps the page
// and the routes from offering what it would refuse.
//
// And every action is asked a fourth question, last: whether the helper is
// busy with a check (409 conflict.host-helper-busy, 13-REVIEW-2 WR-01). The
// helper waits for the check unit, up to its service's 3-min limit, and picks
// up nothing meanwhile; an order placed then would lie in the slot until the
// Box withdrew it after 10 s. hostaction.Box.Busy answers it from the
// helper's own record of the check -- started and not yet done or failed --
// and the check the Box placed last, and GET /api/v1/host carries the same
// answer as actions.busy, which the page turns its buttons off by. Both routes
// ask it where they ask the check's own question, so no token is issued and
// none is spent. Place asks it once more under the slot's lock: a check taken
// between the question and the placement is refused there, with the same 409.

// hostIntentTarget is the Machine field of every host action's confirmation
// intent. A machine id is a UUID, and the inventory's pseudo ids use prefixes
// (unadopted:, adopting:, member:, endpoint:, manual:), none with an "@" -- so
// no node's token can name it (D-08).
const hostIntentTarget = "@host"

// hostActionName is the audit Action and the confirmation's action of one host
// action: host.reboot, host.poweroff, host.restart-service, host.update,
// host.check-update.
func hostActionName(a hostaction.Action) string {
	return "host." + string(a)
}

// hostTypedPhrase says, for every host action the host confirm route will
// issue a token for, whether the operator has to type the hostname first. All
// five do, because HACT-05 wants it for every host action and ROADMAP
// criterion 1 lists the check among the five. For the four that take the host
// or the service away, D-09's reason holds: the node rule -- reboot and
// shutdown without typing, so that typing still means something where it
// matters -- does not carry over, because there is exactly one host and it is
// the machine this page runs on; after the click it is gone. The check takes
// nothing away; for it the reason is that root acts on the host on the page's
// behalf, with network and the release token.
//
// It is its own table and deliberately not merged into typedPhrase: that one
// is the node confirm route's, and an entry there would make
// POST /api/v1/machines/{id}/confirm issue host-action tokens.
// TestEveryHostActionRequiresTyping holds both halves.
var hostTypedPhrase = map[string]bool{
	hostActionName(hostaction.Reboot):         true,
	hostActionName(hostaction.Poweroff):       true,
	hostActionName(hostaction.RestartService): true,
	hostActionName(hostaction.Update):         true,
	hostActionName(hostaction.CheckUpdate):    true,
}

// HostRoutes serves the host page's read, the host confirm route and the five
// host action routes.
func HostRoutes(d httpapi.Deps) []httpapi.Route {
	routes := []httpapi.Route{
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/host",
			RequiresSession: true,
			MinRole:         model.RoleReader,
			Handler:         handler(readHost(d)),
		},
		{
			Method:          http.MethodPost,
			Pattern:         "/api/v1/host/confirm",
			RequiresSession: true,
			MinRole:         model.RoleOperator,
			Action:          "action.confirm",
			Handler:         handler(confirmHostAction(d)),
		},
	}
	for _, a := range hostaction.Actions() {
		routes = append(routes, httpapi.Route{
			Method:          http.MethodPost,
			Pattern:         "/api/v1/host/actions/" + string(a),
			RequiresSession: true,
			MinRole:         model.RoleOperator,
			Destructive:     true,
			Action:          hostActionName(a),
			Handler:         handler(hostAction(d, a)),
		})
	}
	return routes
}

// hostActionsConfigured is the problem for an instance that cannot take host
// orders, or nil.
func hostActionsConfigured(d httpapi.Deps) *httpapi.Problem {
	switch {
	case d.Host == nil:
		return httpapi.Upstream("upstream.host-unavailable",
			"This instance was started without a host reader, so it cannot take host actions.")
	case d.HostActions == nil:
		return httpapi.Upstream("upstream.host-unavailable",
			"This instance was started without host actions.")
	case d.Confirmer == nil:
		return httpapi.Upstream("upstream.host-unavailable",
			"This instance was started without confirmations, so it cannot take host actions.")
	case d.Auth == nil:
		return httpapi.Upstream("upstream.host-unavailable",
			"This instance was started without sessions, so it cannot take host actions.")
	}
	return nil
}

// hostIntent is what a host token is issued for and checked against: the
// action, the host, and the session that typed the hostname. The session is a
// digest of its id: the id is a credential, and the intent is only ever
// hashed, but it need not be there even so. ok is false when the request
// carries no session, which a route that requires one never sees.
func hostIntent(d httpapi.Deps, r *http.Request, action string) (in jobs.Intent, ok bool) {
	sid := d.Auth.SessionID(r.Context())
	if sid == "" {
		return jobs.Intent{}, false
	}
	sum := sha256.Sum256([]byte(sid))
	return jobs.Intent{
		Action:  action,
		Machine: hostIntentTarget,
		Params:  map[string]string{"session": hex.EncodeToString(sum[:])},
	}, true
}

// The details of the refusals, the UI-SPEC's server sentences verbatim.
const (
	hostInContainerDetail   = "Host actions are only available with the systemd installation."
	hostHelperMissingDetail = "The holzkube-manager-host helper is not installed, so no order was placed. The Host page says what to install."
	// hostHelperOutdatedDetail is the check's own refusal: the four older
	// orders still go through.
	hostHelperOutdatedDetail = "The installed holzkube-manager-host helper does not carry out an update check yet, so no order was placed. The Host page says what to reinstall."
	// hostHelperBusyDetail begins with the page's own reason line for a
	// running check (REASON.checkRunning in web/src/components/HostActions.tsx),
	// so the button's reason and the refusal say the same thing.
	hostHelperBusyDetail = "An update check is running; wait for it to finish. No order was placed."
)

// confirmHostAction hands out a token for one host action, to somebody who
// typed this machine's hostname.
func confirmHostAction(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := hostActionsConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}
		// Refused before the body is read: see "Refused before anything else"
		// above. In a container first, where installing is the wrong advice.
		if d.Host.InContainer() {
			httpapi.WriteProblem(w, r, httpapi.Conflict(httpapi.CodeHostInContainer, hostInContainerDetail))
			return
		}
		if len(d.HostActions.Missing()) > 0 {
			httpapi.WriteProblem(w, r, httpapi.Conflict(httpapi.CodeHostHelperMissing, hostHelperMissingDetail))
			return
		}

		var body struct {
			Action string `json:"action"`
			// Typed is what the operator typed into the dialog. It is checked
			// here, once, so that a client that skipped the dialog gets no
			// token at all.
			Typed string `json:"typed"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			httpapi.WriteProblem(w, r, decodeProblem(err))
			return
		}

		needsPhrase, known := hostTypedPhrase[body.Action]
		if !known {
			httpapi.WriteProblem(w, r, httpapi.Validation(
				"This instance issues host confirmations for its five host actions and that is not one of them.",
				httpapi.FieldError{Field: "action", Reason: "not a confirmable host action"}))
			return
		}
		// The check, before anything is typed against: no token for an order
		// the installed helper would refuse.
		if body.Action == hostActionName(hostaction.CheckUpdate) && len(d.HostActions.Outdated()) > 0 {
			httpapi.WriteProblem(w, r, httpapi.Conflict(httpapi.CodeHostHelperOutdated, hostHelperOutdatedDetail))
			return
		}
		// Every action, after the check's own refusal: no token while the
		// helper waits for a check and can pick up nothing else.
		if d.HostActions.Busy() {
			httpapi.WriteProblem(w, r, httpapi.Conflict(httpapi.CodeHostHelperBusy, hostHelperBusyDetail))
			return
		}

		if needsPhrase {
			// Read now, not from the page's last answer: the name the operator
			// typed is compared with the name the machine has.
			hostname, err := d.Host.Hostname()
			if err != nil || hostname == "" {
				httpapi.WriteProblem(w, r, httpapi.Validation(
					"The hostname could not be read, so there is nothing to type to confirm a host action."))
				return
			}
			if strings.TrimSpace(body.Typed) != hostname {
				httpapi.WriteProblem(w, r, httpapi.Validation(
					"Type this machine's hostname exactly to confirm.",
					httpapi.FieldError{Field: "typed", Reason: "does not match the hostname"}))
				return
			}
		}

		intent, ok := hostIntent(d, r, body.Action)
		if !ok {
			httpapi.WriteProblem(w, r, httpapi.Upstream("upstream.host-unavailable",
				"This request carries no session to bind the confirmation to."))
			return
		}
		// Single use (WR-05): IssueOnce's token carries a nonce, so two
		// confirmations in one second are two tokens, each for one order.
		token, expires := d.Confirmer.IssueOnce(intent)

		writeJSON(w, http.StatusOK, map[string]any{
			"token":   token,
			"expires": expires.Format(time.RFC3339),
			"action":  body.Action,
		})
	}
}

// hostAction places the order for one host action.
func hostAction(d httpapi.Deps, a hostaction.Action) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := hostActionsConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}
		// Before the token and before Place: with no helper to pick it up,
		// no order is placed at all.
		if d.Host.InContainer() {
			httpapi.WriteProblem(w, r, httpapi.Conflict(httpapi.CodeHostInContainer, hostInContainerDetail))
			return
		}
		if len(d.HostActions.Missing()) > 0 {
			httpapi.WriteProblem(w, r, httpapi.Conflict(httpapi.CodeHostHelperMissing, hostHelperMissingDetail))
			return
		}
		// The check alone: an older helper refuses it, so it is not placed.
		if a == hostaction.CheckUpdate && len(d.HostActions.Outdated()) > 0 {
			httpapi.WriteProblem(w, r, httpapi.Conflict(httpapi.CodeHostHelperOutdated, hostHelperOutdatedDetail))
			return
		}
		// Every action: while the helper waits for a check it picks up
		// nothing, so nothing is placed for it to leave lying. Before the
		// body and the token, so a refused client keeps its token.
		if d.HostActions.Busy() {
			httpapi.WriteProblem(w, r, httpapi.Conflict(httpapi.CodeHostHelperBusy, hostHelperBusyDetail))
			return
		}

		var body struct {
			Confirmation string `json:"confirmation"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			httpapi.WriteProblem(w, r, decodeProblem(err))
			return
		}

		// The intent is rebuilt from the route and the session, never read
		// out of the token: the action is this route's, the target is the
		// host, and the session is the one this request came with. CheckOnce
		// spends the token: a second order needs the hostname typed again.
		intent, ok := hostIntent(d, r, hostActionName(a))
		if !ok {
			writeJobError(w, r, d, jobs.ErrConfirmationInvalid)
			return
		}
		if err := d.Confirmer.CheckOnce(body.Confirmation, intent); err != nil {
			writeJobError(w, r, d, err)
			return
		}

		order, err := d.HostActions.Place(a)
		if errors.Is(err, hostaction.ErrBusy) {
			// The helper took a check in the moment since Busy above. The
			// token is spent; nothing was placed.
			httpapi.WriteProblem(w, r, httpapi.Conflict(httpapi.CodeHostHelperBusy, hostHelperBusyDetail))
			return
		}
		if errors.Is(err, hostaction.ErrPending) {
			httpapi.WriteProblem(w, r, httpapi.Conflict(httpapi.CodeHostOrderPending,
				"Another host action is still waiting for the helper. Wait for it to be answered, then try again."))
			return
		}
		if err != nil {
			httpapi.WriteInternal(w, r, d.Logger, err)
			return
		}

		// 202: the order is placed and nothing has happened yet. For
		// restart-service and update the helper ends this very process, which
		// is why the answer is written before the helper can act.
		writeJSON(w, http.StatusAccepted, map[string]any{"order": order})
	}
}

func readHost(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Host == nil {
			httpapi.WriteProblem(w, r, httpapi.Upstream("upstream.host-unavailable",
				"This instance was started without a host reader."))
			return
		}
		writeJSON(w, http.StatusOK, d.Host.Read(r.Context()))
	}
}
