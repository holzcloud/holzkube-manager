package kube

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// The wall (2026-09-20).
//
// # What this is for
//
// A screen in the IT office, read from across the room, by somebody who is not
// operating anything. It answers one question -- is everything all right -- and
// it has to answer it in the second somebody glances up.
//
// That makes it a different product from every other screen here, and the
// differences are all consequences of being read from four metres away by
// nobody in particular:
//
//   - ONE REQUEST. A wall makes the same call every few seconds for weeks. Five
//     calls would be five chances for one to fail while the other four painted a
//     confident picture, and nothing on the screen would say which quarter of it
//     was stale.
//   - ONE STATE PER THING, decided here. A tile is a colour; working out what
//     colour from four numbers is arithmetic that must not happen twice, on a
//     screen where nobody will notice it happening differently.
//   - NO ADDRESSES, NO VERSIONS, NO COUNTS NOBODY READS. Names and states. This
//     answer is the one most likely to end up on a screen a visitor can see.
//
// # Stopped is not broken, and unknown is not healthy
//
// Three failures a wall makes if it only has "green" and "red":
//
// A CronJob between runs has no pods. Arithmetic calls that nought of nought and
// paints it red at three in the morning, and by the second week nobody looks at
// the wall any more. A workload somebody deliberately stopped is not a fault
// either -- it is a decision, and it has its own colour so that seeing it is not
// the same as being alarmed by it.
//
// And a node that stopped reporting is NOT a healthy node. It is the one case a
// wall must never paint green, because that is the case a wall exists for.

// State is what a tile is, decided here so that nothing on the wall derives it.
type State string

const (
	// StateOK is running as intended.
	StateOK State = "ok"

	// StateWarn is running, and less of it than there should be. A deployment
	// at two of three, a node that is cordoned: worth seeing, not an outage.
	StateWarn State = "warn"

	// StateDown is not running what it should be running at all.
	StateDown State = "down"

	// StateStopped is a decision rather than a fault: scaled to zero, or
	// suspended. Its own colour so that seeing it is not being alarmed by it.
	StateStopped State = "stopped"

	// StateUnknown is the one a wall must never paint green: nobody is saying.
	StateUnknown State = "unknown"
)

// Tile is one thing on the wall.
type Tile struct {
	// Kind is "Node" or the workload kind, for grouping and for the small
	// label on the tile.
	Kind string `json:"kind"`

	// Namespace is empty for a node.
	Namespace string `json:"namespace"`
	Name      string `json:"name"`

	State State `json:"state"`

	// Detail is the short line under the name: "3 of 3 ready", "cordoned".
	// Written here for the same reason State is -- one sentence, one place.
	Detail string `json:"detail"`
}

// Warning is one recent thing the cluster complained about.
type Warning struct {
	// Object is what it happened to, as the cluster writes it: "Pod/api-7c9".
	Object  string `json:"object"`
	Reason  string `json:"reason"`
	Message string `json:"message"`

	// Count, because a FailedScheduling seen 340 times is a different situation
	// from one seen once, and on a wall that difference is the whole message.
	Count int32 `json:"count"`

	// LastSeen is WHEN, as an instant, not as an age.
	//
	// It used to be called Age and carried the instant anyway, so a wall put
	// "2026-09-20T09:58:00Z" on a television. An age is what somebody four
	// metres away can read -- and it has to be worked out in the browser, not
	// here: this screen keeps the last answer up when a refresh fails, and an
	// age baked in at the server would then freeze at the moment the daemon
	// stopped answering, which is the one moment it must not.
	LastSeen string `json:"last_seen"`
}

// NamespaceTile is a namespace as one tile, coloured by the WORST thing in it.
//
// The roll-up is decided here and not in the browser, for the same reason a
// tile's own state is: it is arithmetic over several things, and arithmetic that
// happens in two places eventually disagrees in one of them -- on a screen
// nobody is standing in front of to notice.
//
// It is the composite pattern Grafana's polystat and its own Kubernetes
// dashboard use: a group of things collapses to one block carrying the worst
// state inside it, and the block names the offender rather than making somebody
// go and look.
type NamespaceTile struct {
	Name  string `json:"name"`
	Total int    `json:"total"`
	State State  `json:"state"`

	// Worst names the thing that decided the colour, as "postgres · 2 of 3
	// ready". Empty when nothing is wrong, because a tile that always carries a
	// sentence is a tile whose sentence nobody reads.
	Worst string `json:"worst"`

	// Stopped is counted separately and never colours the tile: something
	// switched off on purpose is a decision, and a namespace that went amber
	// because somebody paused a job would teach an operator to ignore amber.
	Stopped int `json:"stopped"`
}

// Wall is everything one screen shows, in one answer.
type Wall struct {
	// GeneratedAt is when this was true, so a screen showing it can say how old
	// it is -- and go visibly stale rather than keep showing a green wall that
	// is forty minutes out of date. A wall that cannot go stale is a wall that
	// lies during exactly the incident it exists for.
	GeneratedAt time.Time `json:"generated_at"`

	Nodes     []Tile `json:"nodes"`
	Workloads []Tile `json:"workloads"`

	// Namespaces is the same workloads rolled up, worst-first.
	Namespaces []NamespaceTile `json:"namespaces"`

	// Capacity is the cluster's room: allocatable against requested.
	CPU    Capacity `json:"cpu"`
	Memory Capacity `json:"memory"`
	Pods   Capacity `json:"pods"`

	Warnings []Warning `json:"warnings"`

	// Counts, so a screen with more tiles than it can draw can still say how
	// many there are of each state without drawing them.
	Summary map[State]int `json:"summary"`
}

// MaxWallWarnings bounds the list.
//
// A wall shows the newest few and nothing else: the tenth-oldest warning is not
// something anybody reads from four metres away, and a list that grows without
// bound is a list that pushes the tiles off the screen.
const MaxWallWarnings = 6

// ForTheWall builds everything one screen shows.
//
// Four upstream calls, in one route, deliberately -- see the file comment. The
// namespace narrows the workloads and the warnings; nodes and capacity are the
// cluster's and are never narrowed, because a wall that hid a dead node because
// somebody had picked a namespace would be the worst possible failure of this
// screen.
func (c *Client) ForTheWall(ctx context.Context, namespace string, now time.Time) (Wall, error) {
	out := Wall{
		GeneratedAt: now.UTC(),
		Nodes:       make([]Tile, 0, 8),
		Workloads:   make([]Tile, 0, 32),
		Warnings:    make([]Warning, 0, MaxWallWarnings),
		Summary:     map[State]int{},
	}

	nodes, err := c.Nodes(ctx)
	if err != nil {
		return Wall{}, err
	}
	for _, node := range nodes {
		out.Nodes = append(out.Nodes, nodeTile(node))
	}

	workloads, err := c.Workloads(ctx, namespace)
	if err != nil {
		return Wall{}, err
	}
	for _, workload := range workloads {
		out.Workloads = append(out.Workloads, workloadTile(workload))
	}

	capacity, err := c.Capacity(ctx)
	if err != nil {
		return Wall{}, err
	}
	out.CPU, out.Memory, out.Pods = capacity.CPU, capacity.Memory, capacity.Pods

	events, err := c.Events(ctx, namespace)
	if err != nil {
		return Wall{}, err
	}
	out.Warnings = warningsFrom(events)

	// Worst first, so a screen that cannot draw everything draws the part that
	// matters. Within a state, by name, so a tile does not move between
	// refreshes -- something jumping about on a wall is read as something
	// changing.
	sortTiles(out.Nodes)
	sortTiles(out.Workloads)

	for _, tile := range out.Nodes {
		out.Summary[tile.State]++
	}
	for _, tile := range out.Workloads {
		out.Summary[tile.State]++
	}
	out.Namespaces = rollUp(out.Workloads)
	return out, nil
}

// nodeTile decides what a node is.
//
// NotReady is down rather than warn: a node that is not ready runs nothing, and
// half the point of the wall is that this is visible from the door. Cordoned is
// a decision somebody made, so it is warn and not down -- it is still running
// what it has.
func nodeTile(node Node) Tile {
	tile := Tile{Kind: "Node", Name: node.Name}

	switch {
	case node.Ready == "Unknown" || node.Ready == "":
		// The kubelet stopped reporting. Never green.
		tile.State, tile.Detail = StateUnknown, "not reporting"
	case node.Ready != "True":
		tile.State, tile.Detail = StateDown, "not ready"
	case node.Unschedulable:
		tile.State, tile.Detail = StateWarn, "cordoned"
	default:
		tile.State, tile.Detail = StateOK, "ready"
	}
	return tile
}

// workloadTile decides what a workload is, per kind.
//
// The per-kind split is the same one the workload list makes and for the same
// reason: a CronJob between runs has no pods, and arithmetic over desired and
// ready calls that an outage. On a wall that means red at three in the morning,
// every morning, until nobody looks at the wall.
func workloadTile(w Workload) Tile {
	tile := Tile{
		Kind: string(w.Kind), Namespace: w.Namespace, Name: w.Name,
		Detail: w.Summary,
	}

	switch {
	case w.Stopped:
		// A decision, not a fault. Its own colour.
		tile.State = StateStopped

	case w.Kind == KindCronJob:
		// It has no pods between runs and that is its normal state. Whether it
		// is overdue is a judgement this does not make: the schedule is the
		// operator's and a wall guessing at it would be wrong on every cluster
		// with a monthly job.
		tile.State = StateOK

	case w.Kind == KindJob:
		// A Job that ran and failed is the finding; one still running is fine,
		// and one that finished is fine and is not news.
		if w.Ready == 0 && w.Desired > 0 && w.Summary != "" && isFailedJob(w) {
			tile.State = StateDown
		} else {
			tile.State = StateOK
		}

	case w.Desired == 0:
		// Not stopped by this product and running nothing: a DaemonSet matching
		// no node, a deployment somebody scaled down elsewhere.
		tile.State = StateStopped

	case w.Ready == 0:
		tile.State = StateDown

	case w.Ready < w.Desired:
		tile.State = StateWarn

	default:
		tile.State = StateOK
	}
	return tile
}

// isFailedJob reads the summary the workload list already wrote, rather than
// computing a second opinion from the same numbers.
func isFailedJob(w Workload) bool {
	return len(w.Summary) >= 6 && w.Summary[:6] == "failed"
}

// warningsFrom keeps the newest few warnings.
//
// Warnings only. A wall covered in Normal events is a wall nobody reads, and
// "Scheduled" and "Pulled" are the two commonest events in any cluster.
//
// Walked BACKWARDS, because Events answers newest LAST -- that is how a log
// reads and the sequence is the story there. A wall is not a log: it has room
// for six lines and they have to be the six most recent, so the order is
// reversed here rather than in the thing every other screen shares.
func warningsFrom(events []Event) []Warning {
	out := make([]Warning, 0, MaxWallWarnings)
	for i := len(events) - 1; i >= 0; i-- {
		event := events[i]
		if event.Type != "Warning" {
			continue
		}
		out = append(out, Warning{
			Object:   event.Object,
			Reason:   event.Reason,
			Message:  event.Message,
			Count:    event.Count,
			LastSeen: event.LastSeen,
		})
		if len(out) == MaxWallWarnings {
			break
		}
	}
	return out
}

// stateOrder is worst first. A screen that cannot draw everything has to draw
// the part that matters, and the order is what decides that.
var stateOrder = map[State]int{
	StateDown:    0,
	StateUnknown: 1,
	StateWarn:    2,
	StateStopped: 3,
	StateOK:      4,
}

func sortTiles(tiles []Tile) {
	sort.SliceStable(tiles, func(i, j int) bool {
		if stateOrder[tiles[i].State] != stateOrder[tiles[j].State] {
			return stateOrder[tiles[i].State] < stateOrder[tiles[j].State]
		}
		// By name within a state, so a tile does not move between refreshes.
		// Something jumping about on a wall is read as something changing.
		if tiles[i].Namespace != tiles[j].Namespace {
			return tiles[i].Namespace < tiles[j].Namespace
		}
		return tiles[i].Name < tiles[j].Name
	})
}

// Describe is the one-line summary a screen puts in its corner.
func (w Wall) Describe() string {
	down := w.Summary[StateDown] + w.Summary[StateUnknown]
	if down > 0 {
		return fmt.Sprintf("%d not running", down)
	}
	if warn := w.Summary[StateWarn]; warn > 0 {
		return fmt.Sprintf("%d need attention", warn)
	}
	return "everything is running"
}

// rollUp collapses the workloads into one tile per namespace.
func rollUp(tiles []Tile) []NamespaceTile {
	order := make([]string, 0, 16)
	byName := map[string]*NamespaceTile{}

	for _, tile := range tiles {
		name := tile.Namespace
		if name == "" {
			name = "—"
		}
		into, ok := byName[name]
		if !ok {
			into = &NamespaceTile{Name: name, State: StateOK}
			byName[name] = into
			order = append(order, name)
		}
		into.Total++

		if tile.State == StateStopped {
			into.Stopped++
			continue
		}
		// Worse wins, and the tile keeps the sentence of whatever decided it.
		if stateOrder[tile.State] < stateOrder[into.State] {
			into.State = tile.State
			into.Worst = tile.Name + " · " + tile.Detail
		}
	}

	out := make([]NamespaceTile, 0, len(order))
	for _, name := range order {
		tile := *byName[name]
		// A namespace where everything is switched off is not green. Green would
		// say "running", and nothing in it is.
		if tile.Stopped == tile.Total && tile.State == StateOK {
			tile.State = StateStopped
		}
		out = append(out, tile)
	}
	// Worst first, then the biggest: on a wall the eye goes to the top left, and
	// what belongs there is whatever is wrong -- then whatever is largest, which
	// is the closest thing to "most important" this can know without being told.
	sort.SliceStable(out, func(i, j int) bool {
		if stateOrder[out[i].State] != stateOrder[out[j].State] {
			return stateOrder[out[i].State] < stateOrder[out[j].State]
		}
		if out[i].Total != out[j].Total {
			return out[i].Total > out[j].Total
		}
		return out[i].Name < out[j].Name
	})
	return out
}
