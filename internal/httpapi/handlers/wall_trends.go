package handlers

import (
	"context"
	"math"
	"sort"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/history"
	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/model"
)

// wallTrends is what the wall draws as curves (2026-09-26, the operator's
// layout "A -- split as today, with history"): the cluster's load over the
// last day, and each node's processor over the last hour.
//
// Both are read from the history the sampler already keeps. Nothing is asked
// of a node here, so a wall refreshing every ten seconds costs the cluster no
// more than it did before it had curves.
type wallTrends struct {
	// Cluster is the average over the cluster's nodes, per minute, for 24 h:
	// "cpu" and "memory", in percent. An average of the nodes that answered
	// in that minute -- a node that was off does not pull the line to zero.
	Cluster map[string][]history.Point `json:"cluster"`

	// Nodes is each node's processor load over the last hour, keyed by the
	// node's name as the wall's own tiles carry it.
	Nodes map[string][]history.Point `json:"nodes"`
}

func trendsForTheWall(ctx context.Context, d httpapi.Deps, cluster model.ClusterID, now time.Time) wallTrends {
	out := wallTrends{Cluster: map[string][]history.Point{}, Nodes: map[string][]history.Point{}}
	if d.History == nil || d.Inventory == nil {
		return out
	}
	machines, err := d.Inventory.MachinesOf(ctx, cluster)
	if err != nil {
		return out
	}

	days := make([]map[string][]history.Point, 0, len(machines))
	for _, m := range machines {
		days = append(days, d.History.Query(history.MachineSubject(m.ID), history.Range24h, now).Series)
		if m.Hostname != "" {
			if cpu := d.History.Query(history.MachineSubject(m.ID), history.Range1h, now).Series["cpu"]; len(cpu) > 0 {
				out.Nodes[m.Hostname] = cpu
			}
		}
	}
	out.Cluster = clusterAverage(days, "cpu", "memory")
	return out
}

// clusterAverage is, per series and per minute, the mean over the nodes that
// have a point at that minute. A node with no point there is left out of the
// mean rather than counted as zero: an hour a worker was off is not an hour
// the cluster was idle.
func clusterAverage(nodes []map[string][]history.Point, series ...string) map[string][]history.Point {
	out := map[string][]history.Point{}
	for _, name := range series {
		byTime := map[float64][2]float64{}
		for _, node := range nodes {
			for _, p := range node[name] {
				acc := byTime[p[0]]
				byTime[p[0]] = [2]float64{acc[0] + p[1], acc[1] + 1}
			}
		}
		if len(byTime) == 0 {
			continue
		}
		points := make([]history.Point, 0, len(byTime))
		for t, acc := range byTime {
			points = append(points, history.Point{t, math.Round(acc[0]/acc[1]*10) / 10})
		}
		sort.Slice(points, func(i, j int) bool { return points[i][0] < points[j][0] })
		out[name] = points
	}
	return out
}
