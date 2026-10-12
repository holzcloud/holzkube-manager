package handlers

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/model"
)

func TestTheWallNamesOnlyACertificateThatNeedsAction(t *testing.T) {
	t.Parallel()

	if wallCertificatesFrom(nil) != nil {
		t.Error("a cluster nobody has read has a certificate line on the wall")
	}
	fine := &model.CertificateHealth{Items: []model.NodeCertificate{
		{Node: "cp-1", Name: "etcd peer", Urgency: "none", DaysLeft: 200},
		{Node: "cp-1", Name: "Kubernetes authority", Urgency: "badge", DaysLeft: 80},
	}}
	if wallCertificatesFrom(fine) != nil {
		t.Error("a badge-level certificate reached the wall; only banner and critical do")
	}

	bad := &model.CertificateHealth{Items: []model.NodeCertificate{
		{Node: "cp-secret-host", Name: "etcd authority", Subject: "secret-subject", Urgency: "critical", DaysLeft: 5},
		{Node: "cp-2", Name: "etcd peer", Urgency: "banner", DaysLeft: 10},
	}}
	w := wallCertificatesFrom(bad)
	if w == nil || w.State != "critical" || w.Name != "etcd authority" || w.Days != 5 {
		t.Fatalf("wall line = %+v, want the soonest critical one", w)
	}
	raw, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"cp-secret-host", "secret-subject", "node", "subject"} {
		if strings.Contains(string(raw), leak) {
			t.Errorf("the wall's public answer carries %q: %s", leak, raw)
		}
	}
}
