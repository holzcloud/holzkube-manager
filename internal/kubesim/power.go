package kubesim

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Switching apps and nodes off and on again (2026-09-26).
//
// What the power model sends that nothing sent before: a DaemonSet's selector
// patch, a disabled mark that is later REMOVED, pod deletes with a grace
// period of zero, and pod lists narrowed by label. Each of them changes this
// server's state, so a test reads back what an action did -- the rule ledger 3
// taught talossim -- rather than that a call was made.

// appSelector is the selector every workload this fake renders carries. It
// matches the labels podTemplate puts on the template, which is the invariant
// the API server itself enforces: a controller's selector must match its own
// template.
func appSelector(name string) *metav1.LabelSelector {
	return &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}}
}

// matchesLabels applies an equality-based label selector, the only form this
// product sends ("app=web,tier=front"). An empty selector matches everything,
// which is what the API server does with one.
func matchesLabels(labels map[string]string, selector string) bool {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return true
	}
	for _, term := range strings.Split(selector, ",") {
		term = strings.TrimSpace(term)
		if key, value, ok := strings.Cut(term, "!="); ok {
			if labels[strings.TrimSpace(key)] == strings.TrimSpace(value) {
				return false
			}
			continue
		}
		key, value, ok := strings.Cut(term, "=")
		if !ok {
			// A bare key: "has this label".
			if _, has := labels[term]; !has {
				return false
			}
			continue
		}
		value = strings.TrimPrefix(value, "=")
		got, has := labels[strings.TrimSpace(key)]
		if !has || got != strings.TrimSpace(value) {
			return false
		}
	}
	return true
}

// recordGrace notes the grace period a pod DELETE asked for. The caller holds
// s.mu.
func (s *Server) recordGrace(key string, r *http.Request) {
	if s.graces == nil {
		s.graces = map[string]*int64{}
	}
	var opts metav1.DeleteOptions
	if r.Body != nil {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<16))
		if err == nil && len(body) > 0 {
			_ = json.Unmarshal(body, &opts)
		}
	}
	if opts.GracePeriodSeconds == nil {
		// client-go may carry it in the query instead of the body.
		if raw := r.URL.Query().Get("gracePeriodSeconds"); raw != "" {
			var n int64
			if _, err := fmt.Sscan(raw, &n); err == nil {
				opts.GracePeriodSeconds = &n
			}
		}
	}
	s.graces[key] = opts.GracePeriodSeconds
}

// DeleteGrace reports the grace period the DELETE of one pod asked for, and
// whether that pod was deleted at all. A nil period is a delete that named
// none, which is the pod's own terminationGracePeriodSeconds.
func (s *Server) DeleteGrace(namespace, name string) (grace *int64, deleted bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	grace, deleted = s.graces[namespace+"/"+name]
	return grace, deleted
}

// PodNames is every pod this server still holds, as "namespace/name".
func (s *Server) PodNames() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.pods))
	for _, p := range s.pods {
		out = append(out, p.Namespace+"/"+p.Name)
	}
	return out
}

// Annotations returns what one object carries, kind in lower case
// ("deployment", "daemonset", "cronjob", ...). It is a copy.
func (s *Server) Annotations(kind, namespace, name string) map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]string{}
	for k, v := range s.annotationsFor(kind, namespace, name) {
		out[k] = v
	}
	return out
}

// NodeSelector returns a DaemonSet's selector as this server now holds it.
func (s *Server) NodeSelector(namespace, name string) map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.daemonSets {
		if d.Namespace == namespace && d.Name == name {
			out := map[string]string{}
			for k, v := range d.NodeSelector {
				out[k] = v
			}
			return out
		}
	}
	return nil
}

// SetNodeReady changes a node's Ready condition, which is what a kubelet that
// stopped reporting -- or started again -- looks like from the API server.
func (s *Server) SetNodeReady(name string, status corev1.ConditionStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.nodes {
		if s.nodes[i].Name == name {
			s.nodes[i].Ready = status
			return nil
		}
	}
	return fmt.Errorf("kubesim: no node named %q", name)
}

// Unschedulable reports whether a node is cordoned, and false for a node this
// server does not know.
func (s *Server) Unschedulable(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, n := range s.nodes {
		if n.Name == name {
			return n.Unschedulable
		}
	}
	return false
}

// renderDaemonSet is one DaemonSet in the shape Kubernetes sends it, list and
// single read alike.
func (s *Server) renderDaemonSet(r DaemonSet) appsv1.DaemonSet {
	s.mu.Lock()
	annotations := s.annotationsFor("daemonset", r.Namespace, r.Name)
	s.mu.Unlock()

	template := podTemplate(r.Name, r.Image)
	if len(r.NodeSelector) > 0 {
		template.Spec.NodeSelector = map[string]string{}
		for k, v := range r.NodeSelector {
			template.Spec.NodeSelector[k] = v
		}
	}
	return appsv1.DaemonSet{
		TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "DaemonSet"},
		ObjectMeta: metav1.ObjectMeta{
			Name: r.Name, Namespace: r.Namespace, Annotations: annotations,
		},
		Spec: appsv1.DaemonSetSpec{Template: template, Selector: appSelector(r.Name)},
		Status: appsv1.DaemonSetStatus{
			DesiredNumberScheduled: r.Scheduled,
			NumberReady:            r.Ready,
		},
	}
}

func (s *Server) writeDaemonSet(w http.ResponseWriter, namespace, name string) {
	s.mu.Lock()
	var found *DaemonSet
	for i := range s.daemonSets {
		if s.daemonSets[i].Namespace == namespace && s.daemonSets[i].Name == name {
			d := s.daemonSets[i]
			found = &d
			break
		}
	}
	s.mu.Unlock()

	if found == nil {
		s.writeStatus(w, http.StatusNotFound, metav1.StatusReasonNotFound, fmt.Sprintf("daemonsets %q not found", name))
		return
	}
	writeJSON(w, http.StatusOK, s.renderDaemonSet(*found))
}

// patchDaemonSet applies the three patches a DaemonSet receives: the
// annotations on its metadata, the restartedAt stamp a rollout restart writes
// on its template, and the node selector a stop adds a key to and a start
// removes it from. A null in either map removes the key, which is what a
// strategic merge patch means by one.
func (s *Server) patchDaemonSet(w http.ResponseWriter, r *http.Request, namespace, name string) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		s.writeStatus(w, http.StatusBadRequest, metav1.StatusReasonBadRequest, err.Error())
		return
	}
	var patch struct {
		Metadata struct {
			Annotations map[string]*string `json:"annotations"`
		} `json:"metadata"`
		Spec struct {
			Template struct {
				Metadata struct {
					Annotations map[string]string `json:"annotations"`
				} `json:"metadata"`
				Spec struct {
					NodeSelector map[string]*string `json:"nodeSelector"`
				} `json:"spec"`
			} `json:"template"`
		} `json:"spec"`
	}
	if err := json.Unmarshal(body, &patch); err != nil {
		s.writeStatus(w, http.StatusBadRequest, metav1.StatusReasonBadRequest, err.Error())
		return
	}

	s.mu.Lock()
	index := -1
	for i := range s.daemonSets {
		if s.daemonSets[i].Namespace == namespace && s.daemonSets[i].Name == name {
			index = i
			break
		}
	}
	if index < 0 {
		s.mu.Unlock()
		s.writeStatus(w, http.StatusNotFound, metav1.StatusReasonNotFound, fmt.Sprintf("daemonsets %q not found", name))
		return
	}
	if len(patch.Spec.Template.Spec.NodeSelector) > 0 {
		selector := map[string]string{}
		for k, v := range s.daemonSets[index].NodeSelector {
			selector[k] = v
		}
		for k, v := range patch.Spec.Template.Spec.NodeSelector {
			if v == nil {
				delete(selector, k)
				continue
			}
			selector[k] = *v
		}
		s.daemonSets[index].NodeSelector = selector
	}
	if stamp := patch.Spec.Template.Metadata.Annotations["kubectl.kubernetes.io/restartedAt"]; stamp != "" {
		if s.restarted == nil {
			s.restarted = map[string]string{}
		}
		s.restarted["daemonset/"+namespace+"/"+name] = stamp
	}
	current := s.daemonSets[index]
	s.mu.Unlock()

	s.storeAnnotations("daemonset", namespace, name, patch.Metadata.Annotations)
	writeJSON(w, http.StatusOK, s.renderDaemonSet(current))
}

// renderJob is one Job in the shape Kubernetes sends it.
//
// A Job that has succeeded and has nothing running carries the Complete
// condition, because that is the Job controller's own verdict and the one the
// product reads: without it a finished Job would look like one waiting to run.
func renderJob(r Job, annotations map[string]string) batchv1.Job {
	suspend := r.Suspended
	job := batchv1.Job{
		TypeMeta: metav1.TypeMeta{APIVersion: "batch/v1", Kind: "Job"},
		ObjectMeta: metav1.ObjectMeta{
			Name: r.Name, Namespace: r.Namespace, Annotations: annotations,
		},
		Spec: batchv1.JobSpec{
			Suspend: &suspend, Template: podTemplate(r.Name, r.Image), Selector: appSelector(r.Name),
		},
		Status: batchv1.JobStatus{
			Succeeded: r.Succeeded, Failed: r.Failed, Active: r.Active,
		},
	}
	if r.Succeeded > 0 && r.Active == 0 {
		job.Status.Conditions = []batchv1.JobCondition{{
			Type: batchv1.JobComplete, Status: corev1.ConditionTrue,
		}}
	}
	return job
}

func (s *Server) writeJob(w http.ResponseWriter, namespace, name string) {
	s.mu.Lock()
	var found *Job
	for i := range s.jobs {
		if s.jobs[i].Namespace == namespace && s.jobs[i].Name == name {
			j := s.jobs[i]
			found = &j
			break
		}
	}
	annotations := s.annotationsFor("job", namespace, name)
	s.mu.Unlock()

	if found == nil {
		s.writeStatus(w, http.StatusNotFound, metav1.StatusReasonNotFound, fmt.Sprintf("jobs %q not found", name))
		return
	}
	writeJSON(w, http.StatusOK, renderJob(*found, annotations))
}
