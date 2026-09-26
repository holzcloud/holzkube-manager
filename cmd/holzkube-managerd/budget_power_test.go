package main

import (
	"github.com/holzcloud/holzkube-manager/internal/httpapi/handlers"
	"github.com/holzcloud/holzkube-manager/internal/power"
)

// The power model's rows (2026-09-26): three reads and twenty-one actions.
//
// They are generated rather than written out twenty-four times, and that is a
// statement about the routes rather than a shortcut: every node action makes
// the same reads before it submits its job, every cluster action the same, and
// what differs between the seven is what the JOB does -- which is on the
// engine's context and inside no request. An app action is the exception, and
// its rows name the worst of the seven.

// nodePowerCalls is what deciding about one node costs: probing it, and for a
// control-plane node that could be taken down, the etcd gate's reads of every
// control-plane node.
func nodePowerCalls() []upstreamCall {
	return []upstreamCall{
		{name: "probe the node: NewClusterClient Version", class: nodeProbeCall},
		{name: "probe the node: Version", class: nodeProbeCall},
		{name: "gate: NewClusterClient Version (per control-plane node)", class: nodeProbeCall},
		{name: "gate: EtcdStatus", class: nodeFastReadCall},
		{name: "gate: EtcdMemberList", class: nodeFastReadCall},
		{name: "gate: EtcdAlarmList", class: nodeFastReadCall},
	}
}

// clusterPowerCalls is what deciding about a cluster costs: one probe of every
// node, all of them at once, so the series is one node's.
func clusterPowerCalls() []upstreamCall {
	return []upstreamCall{
		{name: "probe every node in parallel: NewClusterClient Version", class: nodeProbeCall},
		{name: "probe every node in parallel: Version", class: nodeProbeCall},
	}
}

// appPowerCalls is reaching the API server and reading the app, plus what the
// action writes.
func appPowerCalls(writes ...string) []upstreamCall {
	calls := []upstreamCall{
		{name: "NewClusterClient: Version (finding a control-plane node)", class: nodeProbeCall},
		{name: "COSI Get: the machine configuration, for the API server's address", class: nodeFastReadCall},
		{name: "Kubernetes: read the app", class: kubeCall},
	}
	for _, w := range writes {
		calls = append(calls, upstreamCall{name: "Kubernetes: " + w, class: kubeCall})
	}
	return calls
}

// appWrites is the series each app action adds, at its worst kind.
var appWrites = map[power.Action][]string{
	// A Deployment: the count is read and written down, then the scale
	// subresource read and set.
	power.Stop: {"get the scale", "patch the annotation (remember the count)",
		"get the scale subresource", "put the scale subresource to zero"},
	// The stop, then the app's selector read and its pods listed and each one
	// deleted with no grace period.
	power.ForceStop: {"get the scale", "patch the annotation (remember the count)",
		"get the scale subresource", "put the scale subresource to zero",
		"read the app's selector", "list its pods", "delete each pod (per pod)"},
	power.Start: {"get the object (what was written down)", "get the scale subresource",
		"put the scale subresource"},
	// The mark, then the stop.
	power.Disable: {"patch the disabled mark", "get the scale", "patch the annotation (remember the count)",
		"get the scale subresource", "put the scale subresource to zero"},
	power.Enable: {"remove the disabled mark", "get the object (what was written down)",
		"get the scale subresource", "put the scale subresource"},
	power.Restart:      {"patch the pod template's restartedAt"},
	power.ForceRestart: {"read the app's selector", "list its pods", "delete each pod (per pod)"},
}

const appPowerClipping = "Small calls against an API server that answers in milliseconds; each " +
	"ceiling in the sum is a call that is not being answered at all. The per-pod deletes are " +
	"one app's pods -- a handful in the cluster this is for -- and the ceiling is what cuts a " +
	"pathological one, with the error saying how far it got."

func powerRouteBudgets() []routeBudget {
	rows := []routeBudget{
		{
			route:         "GET /api/v1/machines/{id}/power",
			calls:         nodePowerCalls(),
			routeDeadline: handlers.PowerRouteBudget,
			verdict:       withinBudget,
			clipping:      uncut,
			why: "The report asks the node whether it answers, and -- only for a control-plane node " +
				"that is up -- asks the etcd gate whether the cluster can spare it. The gate's reads " +
				"are declared once, as the upgrade rows declare them; the ceiling covers a " +
				"homelab-sized control plane of them.",
		},
		{
			route:         "GET /api/v1/clusters/{id}/power",
			calls:         clusterPowerCalls(),
			routeDeadline: handlers.PowerRouteBudget,
			verdict:       withinBudget,
			clipping:      uncut,
			why: "Every node is probed concurrently, so a cluster with nodes that are off costs one " +
				"probe budget and not one per node. The etcd gate is not asked: a cluster stop " +
				"takes the whole control plane down on purpose.",
		},
		{
			route:         "GET /api/v1/clusters/{id}/kubernetes/apps/{namespace}/{kind}/{name}/power",
			calls:         appPowerCalls(),
			routeDeadline: handlers.PowerRouteBudget,
			verdict:       withinBudget,
			clipping:      uncut,
			why:           "One read of the app, after the two Talos calls every Kubernetes route makes to find the API server.",
		},
	}

	for _, a := range power.Actions() {
		rows = append(rows,
			routeBudget{
				route:         "POST /api/v1/machines/{id}/power/" + string(a),
				calls:         nodePowerCalls(),
				routeDeadline: handlers.PowerRouteBudget,
				verdict:       withinBudget,
				clipping:      uncut,
				why: "The same reads as the report -- the action is checked against the rules the " +
					"screen showed -- and then a job is submitted. Every call the " + string(a) +
					" itself makes happens inside that job, on the engine's context.",
			},
			routeBudget{
				route:         "POST /api/v1/clusters/{id}/power/" + string(a),
				calls:         clusterPowerCalls(),
				routeDeadline: handlers.PowerRouteBudget,
				verdict:       withinBudget,
				clipping:      uncut,
				why: "The report's parallel probes, then a job. Waking, waiting, draining and " +
					"rebooting are all the job's, never this request's.",
			},
			routeBudget{
				route: "POST /api/v1/clusters/{id}/kubernetes/apps/{namespace}/{kind}/{name}/power/" +
					string(a),
				calls:             appPowerCalls(appWrites[a]...),
				routeDeadline:     handlers.PowerRouteBudget,
				verdict:           withinBudget,
				clipping:          clipped,
				clippingRationale: appPowerClipping,
				why: "An app action is synchronous: the report's read, then what " + string(a) +
					" writes, at its most expensive kind.",
			},
		)
	}
	return rows
}
