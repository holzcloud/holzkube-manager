package configdrift

import (
	"sort"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/model"
)

// HealthFor is what the card of one cluster says about the configuration of its
// nodes, at now. It is derived from the readings held in memory and the nodes of
// the last pass, and never stored; no node is asked and the store is not
// touched.
//
// It returns nil until a pass has been through the cluster, which the card says
// as "not read yet". Nil is not "no drift", and nothing here may turn into it.
//
// The counts come from the readings and not from numbers the pass kept, because
// a poke reads one node between two passes and the card must show what that
// reading found, not what the pass before it saw. A node of the pass with no
// reading (it was forgotten since) is "not-read".
func (s *Service) HealthFor(c model.Cluster, _ time.Time) *model.ConfigDriftHealth {
	s.mu.Lock()
	p, ok := s.passes[c.ID]
	if !ok {
		s.mu.Unlock()
		return nil
	}
	checked := p.at
	items := make([]model.NodeConfigDrift, 0, len(p.members))
	for id, host := range p.members {
		rd, read := s.readings[id]
		if !read || rd.cluster != c.ID {
			items = append(items, model.NodeConfigDrift{
				Machine: id, Node: host, State: model.ConfigStateNotRead, Sentence: sentenceNotRead,
			})
			continue
		}
		if rd.at.After(checked) {
			checked = rd.at
		}
		node := host
		if node == "" {
			node = rd.node
		}
		items = append(items, model.NodeConfigDrift{
			Machine: id, Node: node, State: string(rd.state),
			Since: rd.since, ReadAt: rd.lastGood, Sentence: listSentence(rd),
		})
	}
	s.mu.Unlock()

	sort.Slice(items, func(i, j int) bool {
		if items[i].Node != items[j].Node {
			return items[i].Node < items[j].Node
		}
		return items[i].Machine < items[j].Machine
	})

	h := &model.ConfigDriftHealth{CheckedAt: checked, Nodes: len(items), Items: items}
	for _, it := range items {
		if it.Read() {
			h.ReadNodes++
		}
		if it.Drifting() {
			h.Drifted++
		}
	}
	return h
}

// listSentence is the sentence a node carries in the cluster list. That list is
// readable by a Reader, so a sentence that quotes an error does not go in as it
// is: the error text of a failed reading can name an address, and the one of a
// failed store a path in the data directory. The sentences of Detail keep them,
// behind the operator's route.
func listSentence(r *reading) string {
	switch r.state {
	case StateUnreadable:
		return sentenceUnreadable("The node could not be read at the last attempt.", r.lastGood)
	case StateBaselineDamaged:
		return "The stored baseline cannot be trusted, so this node is not compared until it is dealt with."
	}
	return r.sentence
}
