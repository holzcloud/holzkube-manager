package configdrift

import (
	"sort"
	"strconv"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/model"
)

// BannerAfter is how long a drift runs unconfirmed before the banner.
const BannerAfter = 24 * time.Hour

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
func (s *Service) HealthFor(c model.Cluster, now time.Time) *model.ConfigDriftHealth {
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
	oldest := -1
	for i, it := range items {
		if it.Read() {
			h.ReadNodes++
		}
		if it.Drifting() {
			h.Drifted++
			if oldest < 0 || it.Since.Before(items[oldest].Since) {
				oldest = i
			}
		}
	}

	// The banner (D-06): a drift that has run unconfirmed for BannerAfter,
	// counted from its start. The start is the record's (reading.since mirrors
	// DriftSince and survives a failed reading and a restart), never the time of
	// the last reading, and `now` is a parameter so that nothing here waits.
	// Confirming ends the drift and with it the banner; a node that stopped
	// answering keeps its start and so keeps the banner.
	if oldest >= 0 && now.Sub(items[oldest].Since) >= BannerAfter {
		h.Banner = true
		h.Warning = bannerSentence(c.Name, items[oldest], h.Drifted-1)
	}
	return h
}

// bannerSentence is the server's sentence for the banner. It names the oldest
// drift and counts the others, and it names no path, no value and no digest: the
// banner is on every page, and the difference is behind the operator's route.
func bannerSentence(cluster string, oldest model.NodeConfigDrift, others int) string {
	out := "The configuration of " + oldest.Node + " (" + cluster + ") has differed since " + when(oldest.Since) +
		" from what holzkube-manager last knew. Look at it on the node's page, confirm it, or find out who changed it."
	switch others {
	case 0:
	case 1:
		out += " 1 other node differs as well."
	default:
		out += " " + strconv.Itoa(others) + " other nodes differ as well."
	}
	return out
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
