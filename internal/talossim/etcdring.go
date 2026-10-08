package talossim

import "sync"

// EtcdRing is the etcd membership several simulated control-plane nodes share.
//
// Without it each simulated node is a cluster of one and lists only itself,
// which cannot express the questions a removal asks: is this node still a
// member as seen from a peer, and did leaving take it out of everyone's list.
// A ring answers them the way a real etcd does -- one membership, read through
// any member.
type EtcdRing struct {
	mu    sync.Mutex
	hosts []string
}

// NewEtcdRing returns an empty ring.
func NewEtcdRing() *EtcdRing { return &EtcdRing{} }

// Members lists the hostnames in the ring.
func (r *EtcdRing) Members() []string { return r.members() }

// nil-safe: a node without a ring calls these on a nil pointer.

func (r *EtcdRing) join(host string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, h := range r.hosts {
		if h == host {
			return
		}
	}
	r.hosts = append(r.hosts, host)
}

func (r *EtcdRing) leave(host string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.hosts[:0:0]
	for _, h := range r.hosts {
		if h != host {
			out = append(out, h)
		}
	}
	r.hosts = out
}

func (r *EtcdRing) members() []string {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.hosts...)
}
