package kube

import (
	"context"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Who may do what in this cluster (2026-09-20).
//
// # Why a list of roles is not the answer
//
// `kubectl get rolebindings` is a list of names. The questions somebody has about
// RBAC are all about whether an arrangement WORKS, and every one of them is
// answered by two objects disagreeing:
//
//   - "Is this binding doing anything?" A RoleBinding naming a Role that does not
//     exist grants nothing at all, and looks exactly like one that grants
//     everything it was written for. Nothing warns; RBAC has no referential
//     integrity, deliberately, so that a binding can be written before its role.
//   - "Is this service account real?" A binding naming a ServiceAccount that was
//     never created, or was deleted with the binding left behind, is the same
//     shape of nothing.
//   - "Who is an administrator here?" That is not a field. It is every subject
//     reachable through a binding to a role with wildcard verbs on wildcard
//     resources, and `cluster-admin` is only the best-known of those.
//   - "What is this service account allowed to do?" A pod runs as one, and
//     finding out otherwise means reading every binding in the cluster.
//
// # The wildcard is the finding, not cluster-admin by name
//
// Checking for the NAME `cluster-admin` would miss every hand-written role with
// `verbs: ["*"]` on `resources: ["*"]`, which is the same power under a name
// nobody recognises — and the usual way somebody grants it by accident. So the
// rules are read.
//
// # Nothing here writes
//
// A wrong RBAC change locks the operator, and this daemon, out of the cluster.
// The manifest path exists for anybody who means to change a binding, with a plan
// shown first.

// Subject is one thing a binding grants to.
type Subject struct {
	// Kind is User, Group or ServiceAccount.
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`

	// Exists is meaningful only for a ServiceAccount: a User or a Group is the
	// identity provider's business and this cluster has no list of them. Nil
	// would be the honest type; instead Checkable says whether Exists means
	// anything, because a screen showing "does not exist" for every OIDC user
	// would teach that the column is noise.
	Checkable bool `json:"checkable"`
	Exists    bool `json:"exists"`
}

// BindingSummary is one RoleBinding or ClusterRoleBinding, and whether it does
// anything.
type BindingSummary struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`

	// RoleKind is Role or ClusterRole; RoleName what it points at.
	RoleKind string `json:"role_kind"`
	RoleName string `json:"role_name"`

	// RoleExists is false for a binding that grants nothing because the role it
	// names is not there. RBAC has no referential integrity on purpose, so
	// nothing else in the cluster will say this.
	RoleExists bool `json:"role_exists"`

	Subjects []Subject `json:"subjects"`

	// Administrative is true when the role behind it has wildcard verbs on
	// wildcard resources -- whatever it is called. Checking for the name
	// cluster-admin would miss every hand-written role with the same power.
	Administrative bool `json:"administrative"`

	// Summary is what this binding actually does, in words.
	Summary string `json:"summary"`

	// Healthy is false when the binding grants nothing: a missing role, a
	// missing service account, or no subjects at all.
	Healthy   bool   `json:"healthy"`
	CreatedAt string `json:"created_at"`
}

// RoleSummary is one Role or ClusterRole, in what it permits.
type RoleSummary struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`

	// Rules in readable lines: "get, list, watch on pods, services".
	Rules []string `json:"rules"`

	// Administrative is wildcard verbs on wildcard resources.
	Administrative bool `json:"administrative"`

	// Bound is how many bindings point at it. Zero is a role that permits
	// nothing because nothing grants it -- ordinary for the cluster's built-in
	// roles, and worth seeing for one somebody wrote.
	Bound int `json:"bound"`

	// BuiltIn marks a role Kubernetes ships. There are about seventy of them and
	// they are not what somebody is looking for, so a screen can fold them away
	// without this having to decide.
	BuiltIn   bool   `json:"built_in"`
	CreatedAt string `json:"created_at"`
}

// ServiceAccountSummary is one account and what runs as it.
type ServiceAccountSummary struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`

	// UsedBy is every running pod that runs as this account. The "default"
	// account of a namespace is used by every pod that names none, which is how
	// a permission granted to it reaches things nobody intended.
	UsedBy []string `json:"used_by"`

	// Bindings is how many bindings grant to it, and whether any of them is
	// administrative.
	Bindings       int  `json:"bindings"`
	Administrative bool `json:"administrative"`

	Notice    string `json:"notice"`
	CreatedAt string `json:"created_at"`
}

// AccessControl is the cluster's RBAC as the questions somebody has about it.
type AccessControl struct {
	Bindings []BindingSummary        `json:"bindings"`
	Roles    []RoleSummary           `json:"roles"`
	Accounts []ServiceAccountSummary `json:"accounts"`

	// Administrators is every subject that reaches wildcard-on-wildcard through
	// some binding, said as "ServiceAccount ci/deployer". It is not a field
	// anywhere in Kubernetes and it is the first thing anybody wants to know.
	Administrators []string `json:"administrators"`

	Notice string `json:"notice"`
}

// AccessControl reads the cluster's roles, bindings and service accounts.
//
// The namespace narrows the namespaced objects. ClusterRoles and
// ClusterRoleBindings are always included: a cluster-wide grant is not something
// a namespace filter should hide, and it is the grant that matters most.
func (c *Client) AccessControl(ctx context.Context, namespace string) (AccessControl, error) {
	out := AccessControl{
		Notice: "A binding that names a role or a service account which does not exist grants " +
			"nothing, and looks exactly like one that works: RBAC has no referential integrity, " +
			"deliberately, so that a binding may be written before its role. Administrative means " +
			"wildcard verbs on wildcard resources, whatever the role is called -- looking for the " +
			"name cluster-admin would miss every hand-written role with the same power.",
	}

	roles, err := c.cs.RbacV1().Roles(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return AccessControl{}, fmt.Errorf("kube: listing roles: %w", err)
	}
	clusterRoles, err := c.cs.RbacV1().ClusterRoles().List(ctx, metav1.ListOptions{})
	if err != nil {
		return AccessControl{}, fmt.Errorf("kube: listing cluster roles: %w", err)
	}

	// Indexed by "kind/namespace/name" so a binding can ask two questions of it:
	// does the role exist, and is it administrative.
	type roleFacts struct {
		exists         bool
		administrative bool
	}
	facts := map[string]roleFacts{}
	rows := map[string]*RoleSummary{}

	for i := range roles.Items {
		role := &roles.Items[i]
		key := "Role/" + role.Namespace + "/" + role.Name
		admin := rulesAreAdministrative(role.Rules)
		facts[key] = roleFacts{exists: true, administrative: admin}
		rows[key] = &RoleSummary{
			Kind: "Role", Namespace: role.Namespace, Name: role.Name,
			Rules: ruleLines(role.Rules), Administrative: admin,
			BuiltIn:   isBuiltInRole(role.Labels, role.Name),
			CreatedAt: stamp(role.CreationTimestamp),
		}
	}
	for i := range clusterRoles.Items {
		role := &clusterRoles.Items[i]
		key := "ClusterRole//" + role.Name
		admin := rulesAreAdministrative(role.Rules)
		facts[key] = roleFacts{exists: true, administrative: admin}
		rows[key] = &RoleSummary{
			Kind: "ClusterRole", Name: role.Name,
			Rules: ruleLines(role.Rules), Administrative: admin,
			BuiltIn:   isBuiltInRole(role.Labels, role.Name),
			CreatedAt: stamp(role.CreationTimestamp),
		}
	}

	accounts, err := c.cs.CoreV1().ServiceAccounts(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return AccessControl{}, fmt.Errorf("kube: listing service accounts: %w", err)
	}
	realAccount := map[string]bool{}
	for _, account := range accounts.Items {
		realAccount[account.Namespace+"/"+account.Name] = true
	}

	bindings, err := c.cs.RbacV1().RoleBindings(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return AccessControl{}, fmt.Errorf("kube: listing role bindings: %w", err)
	}
	clusterBindings, err := c.cs.RbacV1().ClusterRoleBindings().List(ctx, metav1.ListOptions{})
	if err != nil {
		return AccessControl{}, fmt.Errorf("kube: listing cluster role bindings: %w", err)
	}

	administrators := map[string]bool{}
	accountBindings := map[string]int{}
	accountAdmin := map[string]bool{}

	addBinding := func(
		kind, bindingNamespace, name string, ref rbacv1.RoleRef,
		subjects []rbacv1.Subject, created metav1.Time,
	) {
		key := ref.Kind + "/" + refNamespace(ref, bindingNamespace) + "/" + ref.Name
		known := facts[key]

		row := BindingSummary{
			Kind: kind, Namespace: bindingNamespace, Name: name,
			RoleKind: ref.Kind, RoleName: ref.Name,
			RoleExists: known.exists, Administrative: known.administrative,
			Healthy:   true,
			CreatedAt: stamp(created),
		}
		if row.RoleExists {
			if found := rows[key]; found != nil {
				found.Bound++
			}
		}

		missing := 0
		for _, subject := range subjects {
			entry := Subject{Kind: subject.Kind, Namespace: subject.Namespace, Name: subject.Name}
			if subject.Kind == rbacv1.ServiceAccountKind {
				// The only kind this cluster can check. A User or a Group lives
				// in the identity provider and no cluster has a list of them.
				entry.Checkable = true
				entry.Exists = realAccount[subject.Namespace+"/"+subject.Name]
				if !entry.Exists {
					missing++
				}
				accountKey := subject.Namespace + "/" + subject.Name
				accountBindings[accountKey]++
				if known.administrative {
					accountAdmin[accountKey] = true
				}
			}
			if known.administrative && known.exists {
				administrators[subject.Kind+" "+subjectName(subject)] = true
			}
			row.Subjects = append(row.Subjects, entry)
		}

		switch {
		case !row.RoleExists:
			// The finding: it grants nothing and nothing in the cluster says so.
			row.Healthy = false
			row.Summary = "It names " + ref.Kind + " " + ref.Name +
				", which does not exist, so it grants nothing at all. RBAC does not check this: a " +
				"binding may be written before its role, and one written after its role was removed " +
				"looks the same."
		case len(subjects) == 0:
			row.Healthy = false
			row.Summary = "It has no subjects, so it grants " + ref.Name + " to nobody."
		case missing == len(subjects):
			row.Healthy = false
			row.Summary = "Every service account it names is missing, so it grants nothing."
		case missing > 0:
			row.Healthy = false
			row.Summary = fmt.Sprintf("%d of its %d subjects are service accounts that do not "+
				"exist, so that part of it grants nothing.", missing, len(subjects))
		case row.Administrative:
			row.Summary = "Grants " + ref.Name + ", which permits every verb on every resource."
		default:
			row.Summary = "Grants " + ref.Kind + " " + ref.Name + "."
		}
		out.Bindings = append(out.Bindings, row)
	}

	for i := range bindings.Items {
		b := &bindings.Items[i]
		addBinding("RoleBinding", b.Namespace, b.Name, b.RoleRef, b.Subjects, b.CreationTimestamp)
	}
	for i := range clusterBindings.Items {
		b := &clusterBindings.Items[i]
		addBinding("ClusterRoleBinding", "", b.Name, b.RoleRef, b.Subjects, b.CreationTimestamp)
	}

	// What runs as which account. The "default" account matters most: every pod
	// that names none runs as it, so a permission granted there reaches things
	// nobody intended.
	pods, err := c.cs.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return AccessControl{}, fmt.Errorf("kube: listing pods: %w", err)
	}
	runningAs := map[string][]string{}
	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		name := pod.Spec.ServiceAccountName
		if name == "" {
			name = "default"
		}
		key := pod.Namespace + "/" + name
		runningAs[key] = append(runningAs[key], pod.Namespace+"/"+pod.Name)
	}

	for _, account := range accounts.Items {
		key := account.Namespace + "/" + account.Name
		row := ServiceAccountSummary{
			Namespace: account.Namespace, Name: account.Name,
			UsedBy:         runningAs[key],
			Bindings:       accountBindings[key],
			Administrative: accountAdmin[key],
			CreatedAt:      stamp(account.CreationTimestamp),
		}
		sort.Strings(row.UsedBy)
		switch {
		case row.Administrative && account.Name == "default":
			row.Notice = "This is the account every pod in " + account.Namespace +
				" that names none runs as, and it is bound to a role permitting every verb on " +
				"every resource. Anything that can run a pod here can do anything to the cluster."
		case row.Administrative:
			row.Notice = "Bound to a role permitting every verb on every resource."
		case account.Name == "default" && len(row.UsedBy) > 0:
			row.Notice = "The account pods in " + account.Namespace +
				" run as when they name none."
		}
		out.Accounts = append(out.Accounts, row)
	}

	for _, row := range rows {
		out.Roles = append(out.Roles, *row)
	}
	sort.Slice(out.Roles, func(i, j int) bool {
		if out.Roles[i].Kind != out.Roles[j].Kind {
			return out.Roles[i].Kind < out.Roles[j].Kind
		}
		return out.Roles[i].Name < out.Roles[j].Name
	})

	for who := range administrators {
		out.Administrators = append(out.Administrators, who)
	}
	sort.Strings(out.Administrators)

	return out, nil
}

// rulesAreAdministrative reports wildcard verbs on wildcard resources.
//
// One rule has to carry all three wildcards -- verb, group and resource -- rather
// than the set of rules carrying them between them: "* verbs on configmaps" and
// "get on *" are both ordinary, and a check that ORed them would report half the
// cluster's built-in roles as administrative and therefore report nothing.
//
// A rule granting `nonResourceURLs` is not counted: that is /healthz and /metrics
// and it is not administration of anything in the cluster.
func rulesAreAdministrative(rules []rbacv1.PolicyRule) bool {
	for _, rule := range rules {
		if len(rule.Resources) == 0 {
			continue
		}
		if contains(rule.Verbs, "*") && contains(rule.APIGroups, "*") && contains(rule.Resources, "*") {
			return true
		}
	}
	return false
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// ruleLines is what a role permits, in lines somebody can read.
func ruleLines(rules []rbacv1.PolicyRule) []string {
	out := make([]string, 0, len(rules))
	for _, rule := range rules {
		verbs := strings.Join(rule.Verbs, ", ")
		if contains(rule.Verbs, "*") {
			verbs = "everything"
		}
		switch {
		case len(rule.Resources) > 0:
			resources := strings.Join(rule.Resources, ", ")
			if contains(rule.Resources, "*") {
				resources = "every resource"
			}
			out = append(out, verbs+" on "+resources)
		case len(rule.NonResourceURLs) > 0:
			// Not a resource at all: /healthz, /metrics, /version.
			out = append(out, verbs+" on "+strings.Join(rule.NonResourceURLs, ", "))
		default:
			out = append(out, verbs)
		}
	}
	return out
}

// isBuiltInRole marks the roles Kubernetes ships.
//
// There are about seventy, they are the same on every cluster, and they are not
// what somebody is looking for. The label is the cluster's own marking; the name
// prefixes catch the ones that predate it.
func isBuiltInRole(labels map[string]string, name string) bool {
	if labels["kubernetes.io/bootstrapping"] == "rbac-defaults" {
		return true
	}
	return strings.HasPrefix(name, "system:") || name == "cluster-admin" ||
		name == "admin" || name == "edit" || name == "view"
}

// refNamespace is where a RoleRef points.
//
// A ClusterRole is cluster-scoped whoever names it; a Role is always in the
// binding's own namespace, because a RoleBinding cannot name a Role elsewhere.
func refNamespace(ref rbacv1.RoleRef, bindingNamespace string) string {
	if ref.Kind == "ClusterRole" {
		return ""
	}
	return bindingNamespace
}

func subjectName(subject rbacv1.Subject) string {
	if subject.Kind == rbacv1.ServiceAccountKind && subject.Namespace != "" {
		return subject.Namespace + "/" + subject.Name
	}
	return subject.Name
}
