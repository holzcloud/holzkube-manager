package handlers

import (
	"strconv"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/history"
	"github.com/holzcloud/holzkube-manager/internal/host"
	"github.com/holzcloud/holzkube-manager/internal/httpapi"
)

// wallHost is the machine holzkube-manager runs on, as the wall shows it: one
// tile beside the cluster's nodes, and not one of them (D-14).
//
// What it carries is decided here, because a wall link opens this route and no
// other: whatever is in this struct is public to whoever stands in the
// corridor. A name, a state and a short reason -- no address, no version, no
// error text (T-12-08). An unreadable host says the fixed "not readable",
// never the Unreadable sentences, which name paths and carry the kernel's
// errors. A warning is told from Health.Public, never from Warnings: a
// filesystem's warning names its mount point there, and the data directory's
// can be any path on the machine.
type wallHost struct {
	Name   string           `json:"name"`
	State  host.HealthState `json:"state"`
	Reason string           `json:"reason"`
}

// wallHostName is the tile's name when the host's own could not be read.
const wallHostName = "holzkube-manager host"

// wallHostNotReadable is the reason of every tile that is unknown.
const wallHostNotReadable = "not readable"

// hostForTheWall is the host's tile from the sampler's last snapshot, or nil
// when there is no host reader or it has not sampled yet -- the wall then
// draws no host tile rather than a grey one that would never go away.
//
// It never reads the host itself: a wall refreshes every ten seconds on every
// screen, and the sampler reads the host every fifteen already (T-12-11).
func hostForTheWall(d httpapi.Deps, now time.Time) *wallHost {
	if d.Host == nil {
		return nil
	}
	s, ok := d.Host.Latest()
	if !ok {
		return nil
	}
	return wallHostFrom(s, now)
}

// wallHostFrom is the tile for one snapshot at now. Pure.
func wallHostFrom(s host.Snapshot, now time.Time) *wallHost {
	out := &wallHost{Name: s.Name}
	if out.Name == "" {
		out.Name = wallHostName
	}

	// D-11: a sampler that stopped is not a host that is fine. Three missed
	// passes and whatever the snapshot said is no longer known -- and a green
	// that outlived its sampler is the one thing this tile must never show.
	if now.Sub(s.At) > 3*history.FineStep {
		out.State, out.Reason = host.HealthUnknown, wallHostNotReadable
		return out
	}

	switch s.Health.State {
	case host.HealthOK:
		out.State, out.Reason = host.HealthOK, "healthy"
	case host.HealthWarn:
		out.State = host.HealthWarn
		if len(s.Health.Public) == 0 {
			// Assess never warns without a sentence; if it ever did, the tile
			// still says something -- the count, which names nothing.
			out.Reason = s.Health.Summary
			break
		}
		out.Reason = s.Health.Public[0]
		if more := len(s.Health.Public) - 1; more > 0 {
			out.Reason += " and " + strconv.Itoa(more) + " more"
		}
	default:
		out.State, out.Reason = host.HealthUnknown, wallHostNotReadable
	}
	return out
}
