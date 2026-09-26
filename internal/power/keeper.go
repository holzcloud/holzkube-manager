package power

import (
	"context"
	"log/slog"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/model"
)

// KeepEvery is how often the keeper looks at disabled nodes.
//
// A minute: a disabled node somebody switched on at the machine is up and
// joining within a minute or two, and the point is to have it cordoned before
// the scheduler has had long to notice it -- not to be first.
const KeepEvery = time.Minute

// Keep runs Sweep every interval until ctx ends. The composition root starts it
// once, after the inventory.
func (s *Service) Keep(ctx context.Context, every time.Duration) {
	if every <= 0 {
		every = KeepEvery
	}
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		s.Sweep(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Sweep is one pass over the disabled nodes.
//
// Disabled means "off, and stays off", and a mark in a store cannot hold a
// power button down. A disabled node that answers -- somebody pressed its
// button, a power cut and a BIOS set to "restore on AC" -- is kept out of the
// scheduler instead: cordoned, and said so in the log, loudly, because it is a
// machine running that the operator decided should not be. It is not switched
// off again. Turning a machine off behind the back of whoever just turned it on
// is a fight this daemon would lose the moment somebody was standing next to
// it, and the log line is what tells them why the node is not taking work.
//
// A node of a disabled cluster counts as disabled. Everything here is best
// effort and repeats: a Kubernetes API that is down this minute is asked again
// the next.
func (s *Service) Sweep(ctx context.Context) {
	machines, err := s.deps.Store.Machines().List(ctx)
	if err != nil {
		s.deps.Logger.Warn("power: could not list machines for the disabled-node sweep", slog.Any("error", err))
		return
	}
	clusters, err := s.deps.Store.Clusters().List(ctx)
	if err != nil {
		s.deps.Logger.Warn("power: could not list clusters for the disabled-node sweep", slog.Any("error", err))
		return
	}
	disabledCluster := map[model.ClusterID]bool{}
	for _, c := range clusters {
		disabledCluster[c.ID] = c.Disabled
	}

	for _, rec := range machines {
		if rec.Cluster == "" || (!rec.Disabled && !disabledCluster[rec.Cluster]) {
			s.forgetUp(rec.ID)
			continue
		}
		if ans, _ := s.probe(ctx, rec.ID); ans != answerYes {
			s.forgetUp(rec.ID)
			continue
		}
		s.keepCordoned(ctx, rec, s.firstSeenUp(rec.ID))
	}
}

func (s *Service) keepCordoned(ctx context.Context, rec model.Machine, first bool) {
	attrs := []any{
		slog.String("machine", string(rec.ID)),
		slog.String("hostname", rec.Hostname),
		slog.String("cluster", string(rec.Cluster)),
	}

	client, err := s.deps.Kube(ctx, rec.Cluster)
	if err != nil {
		if first {
			s.deps.Logger.Warn("power: a disabled node is running; Kubernetes did not answer, so it "+
				"could not be cordoned yet -- it will be tried again", append(attrs, slog.Any("error", err))...)
		}
		return
	}
	node, err := findNode(ctx, client, memberOf(rec, true))
	if err != nil {
		if first {
			s.deps.Logger.Warn("power: a disabled node is running and Kubernetes has no node for it yet; "+
				"it will be cordoned when it registers", append(attrs, slog.Any("error", err))...)
		}
		return
	}
	if node.Unschedulable {
		if first {
			s.deps.Logger.Warn("power: a disabled node is running; it stays cordoned until it is enabled",
				attrs...)
		}
		return
	}
	if err := client.Cordon(ctx, node.Name, true); err != nil {
		s.deps.Logger.Warn("power: a disabled node is running and could not be cordoned",
			append(attrs, slog.Any("error", err))...)
		return
	}
	s.deps.Logger.Warn("power: a disabled node is running; cordoned it so nothing is scheduled onto it "+
		"until it is enabled", attrs...)
}

// firstSeenUp records that a disabled machine answers and reports whether this
// is the first sweep that saw it do so.
func (s *Service) firstSeenUp(id model.MachineID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.seenUp[id] {
		return false
	}
	s.seenUp[id] = true
	return true
}

func (s *Service) forgetUp(id model.MachineID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.seenUp, id)
}
