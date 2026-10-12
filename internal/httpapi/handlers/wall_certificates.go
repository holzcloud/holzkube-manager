package handlers

import (
	"context"

	"github.com/holzcloud/holzkube-manager/internal/certwatch"
	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/model"
)

// wallCertificates is the one certificate on a cluster that has a date to act
// on, as the wall shows it.
//
// A wall link opens this route and no other, so what is here is public to the
// corridor: the role of the certificate and the days it has left. Not the node
// it is on, not its subject and not the sentence the overview uses, which
// names machines. It is absent while nothing is within the banner rung: a wall
// that said "all certificates fine" every ten seconds would be a line nobody
// reads, and the day it changed would be a line nobody notices.
type wallCertificates struct {
	State string `json:"state"`
	Name  string `json:"name"`
	Days  int    `json:"days_left"`
}

// certificatesForTheWall is the worst certificate of the cluster at banner or
// worse, or nil. It reads the last pass of the certificate watch and never the
// nodes (the wall refreshes every ten seconds on every screen).
func certificatesForTheWall(ctx context.Context, d httpapi.Deps, cluster model.ClusterID) *wallCertificates {
	if d.Inventory == nil {
		return nil
	}
	v, err := d.Inventory.Cluster(ctx, cluster)
	if err != nil {
		return nil
	}
	return wallCertificatesFrom(v.NodeCertificates)
}

// wallCertificatesFrom is the pure half.
func wallCertificatesFrom(h *model.CertificateHealth) *wallCertificates {
	if h == nil {
		return nil
	}
	for _, it := range h.Items {
		// Items are soonest first, so the first one at banner or worse is the one
		// to name; an earlier one below banner is not worth a line.
		if it.Urgency == certwatch.Banner || it.Urgency == certwatch.Critical {
			return &wallCertificates{State: it.Urgency, Name: it.Name, Days: it.DaysLeft}
		}
	}
	return nil
}
