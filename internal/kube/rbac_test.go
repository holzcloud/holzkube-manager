package kube_test

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/holzcloud/holzkube-manager/internal/kube"
	"github.com/holzcloud/holzkube-manager/internal/kubesim"
)

// Who may do what (2026-09-20).
//
// Every claim here is one two objects disagreeing makes, and RBAC has no
// referential integrity on purpose -- so nothing in a cluster will say any of
// them. A binding naming a role that is not there grants nothing and looks
// exactly like one that grants everything it was written for.

func rbacCluster(t *testing.T) (*kubesim.Server, *kube.Client) {
	t.Helper()

	return newCluster(t, kubesim.Options{
		ServiceAccounts: []kubesim.ServiceAccount{
			{Namespace: "default", Name: "default"},
			{Namespace: "ci", Name: "deployer"},
			{Namespace: "web", Name: "reader"},
		},
		Roles: []kubesim.Role{
			// Administrative under a name nobody would recognise: this is the
			// case that makes checking for "cluster-admin" by name useless.
			{Name: "platform-operator", Verbs: []string{"*"}, APIGroups: []string{"*"},
				Resources: []string{"*"}},
			// Wildcards spread across rules are ORDINARY. Two roles, because one
			// rule each is what the fake renders.
			{Name: "configmap-editor", Verbs: []string{"*"}, APIGroups: []string{""},
				Resources: []string{"configmaps"}},
			{Name: "universal-reader", Verbs: []string{"get"}, APIGroups: []string{"*"},
				Resources: []string{"*"}},
			// Not administration of anything in the cluster.
			{Name: "health-checker", Verbs: []string{"get"}, NonResourceURLs: []string{"/healthz"}},
			{Namespace: "web", Name: "pod-reader", Verbs: []string{"get", "list"},
				APIGroups: []string{""}, Resources: []string{"pods"}},
			{Name: "cluster-admin", Verbs: []string{"*"}, APIGroups: []string{"*"},
				Resources: []string{"*"}, BuiltIn: true},
		},
		RoleBindings: []kubesim.RoleBinding{
			{Name: "platform-team", RoleKind: "ClusterRole", RoleName: "platform-operator",
				Subjects: []kubesim.BindingSubject{
					{Kind: "ServiceAccount", Namespace: "ci", Name: "deployer"},
					{Kind: "User", Name: "admin@example.com"},
				}},
			// Names a role that does not exist: grants nothing, looks fine.
			{Namespace: "web", Name: "web-readers", RoleKind: "Role", RoleName: "pod-readr",
				Subjects: []kubesim.BindingSubject{
					{Kind: "ServiceAccount", Namespace: "web", Name: "reader"},
				}},
			// Names a service account that was never created.
			{Namespace: "web", Name: "ghost-binding", RoleKind: "Role", RoleName: "pod-reader",
				Subjects: []kubesim.BindingSubject{
					{Kind: "ServiceAccount", Namespace: "web", Name: "deleted-long-ago"},
				}},
			// No subjects at all.
			{Namespace: "web", Name: "nobody", RoleKind: "Role", RoleName: "pod-reader"},
		},
		Pods: []kubesim.Pod{
			{Namespace: "ci", Name: "runner-abc", Phase: corev1.PodRunning, ServiceAccount: "deployer"},
			// Names none, so it runs as "default".
			{Namespace: "default", Name: "bare", Phase: corev1.PodRunning},
		},
	})
}

func bindingsByName(network kube.AccessControl) map[string]kube.BindingSummary {
	out := map[string]kube.BindingSummary{}
	for _, binding := range network.Bindings {
		out[binding.Name] = binding
	}
	return out
}

// TestABindingNamingARoleThatIsNotThereGrantsNothing, and nothing in the cluster
// says so.
func TestABindingNamingARoleThatIsNotThereGrantsNothing(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := rbacCluster(t)

	rbac, err := client.AccessControl(ctx, "")
	if err != nil {
		t.Fatalf("AccessControl: %v", err)
	}
	broken := bindingsByName(rbac)["web-readers"]

	if broken.RoleExists {
		t.Error("a binding naming a role that does not exist is reported as pointing at one")
	}
	if broken.Healthy {
		t.Error("a binding that grants nothing is reported as healthy")
	}
	if !strings.Contains(broken.Summary, "does not exist") {
		t.Errorf("summary = %q, want it to say the role is not there", broken.Summary)
	}
	// And the reason RBAC does not catch it, because otherwise the finding reads
	// like a bug in the cluster.
	if !strings.Contains(broken.Summary, "may be written before its role") {
		t.Errorf("summary = %q, want it to say why RBAC allows this", broken.Summary)
	}
}

// TestABindingNamingAServiceAccountThatIsNotThereGrantsNothing.
func TestABindingNamingAServiceAccountThatIsNotThereGrantsNothing(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := rbacCluster(t)

	rbac, err := client.AccessControl(ctx, "")
	if err != nil {
		t.Fatalf("AccessControl: %v", err)
	}
	ghost := bindingsByName(rbac)["ghost-binding"]

	if !ghost.RoleExists {
		t.Fatal("the role it names does exist; this test is about the subject")
	}
	if ghost.Healthy {
		t.Error("a binding whose only subject is missing is reported as healthy")
	}
	if len(ghost.Subjects) != 1 || ghost.Subjects[0].Exists {
		t.Errorf("subjects = %+v, want the missing account marked", ghost.Subjects)
	}
	if !ghost.Subjects[0].Checkable {
		t.Error("a ServiceAccount subject is not marked checkable")
	}
}

// TestAUserSubjectIsNotReportedAsMissing.
//
// A User or a Group lives in the identity provider and no cluster has a list of
// them. A column saying "does not exist" for every OIDC user would teach that the
// column is noise, and the real finding would then be invisible.
func TestAUserSubjectIsNotReportedAsMissing(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := rbacCluster(t)

	rbac, err := client.AccessControl(ctx, "")
	if err != nil {
		t.Fatalf("AccessControl: %v", err)
	}
	team := bindingsByName(rbac)["platform-team"]
	if !team.Healthy {
		t.Errorf("a binding with a real account and a user is not healthy: %q", team.Summary)
	}
	for _, subject := range team.Subjects {
		if subject.Kind == "User" && subject.Checkable {
			t.Error("a User subject is marked checkable; no cluster has a list of users")
		}
	}
}

// TestAdministrativeIsAboutTheRulesAndNotTheName.
//
// Looking for "cluster-admin" would miss every hand-written role with wildcard
// verbs on wildcard resources -- the same power under a name nobody recognises,
// and the usual way somebody grants it by accident.
func TestAdministrativeIsAboutTheRulesAndNotTheName(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := rbacCluster(t)

	rbac, err := client.AccessControl(ctx, "")
	if err != nil {
		t.Fatalf("AccessControl: %v", err)
	}

	if !bindingsByName(rbac)["platform-team"].Administrative {
		t.Error("a binding to a wildcard role called platform-operator is not reported as administrative")
	}

	// And who those subjects are, which is not a field anywhere in Kubernetes.
	want := map[string]bool{
		"ServiceAccount ci/deployer": true,
		"User admin@example.com":     true,
	}
	for _, who := range rbac.Administrators {
		if !want[who] {
			t.Errorf("administrators contains %q, which is not one", who)
		}
		delete(want, who)
	}
	for missing := range want {
		t.Errorf("administrators is missing %q", missing)
	}
}

// TestWildcardsSpreadAcrossRulesAreNotAdministrative.
//
// "* verbs on configmaps" and "get on every resource" are both ordinary. A check
// that ORed the wildcards would report half the cluster's built-in roles as
// administrative, and a warning everybody sees is a warning nobody reads.
func TestWildcardsSpreadAcrossRulesAreNotAdministrative(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := rbacCluster(t)

	rbac, err := client.AccessControl(ctx, "")
	if err != nil {
		t.Fatalf("AccessControl: %v", err)
	}
	for _, role := range rbac.Roles {
		switch role.Name {
		case "configmap-editor", "universal-reader", "health-checker", "pod-reader":
			if role.Administrative {
				t.Errorf("%s is reported as administrative and is not", role.Name)
			}
		case "platform-operator", "cluster-admin":
			if !role.Administrative {
				t.Errorf("%s permits every verb on every resource and is not reported so", role.Name)
			}
		}
	}
}

// TestTheDefaultAccountBoundToEverythingIsSaidLoudly.
//
// Every pod that names no service account runs as `default`. A permission granted
// there reaches things nobody intended, and anything able to run a pod in the
// namespace then has it.
func TestTheDefaultAccountBoundToEverythingIsSaidLoudly(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{
		ServiceAccounts: []kubesim.ServiceAccount{{Namespace: "default", Name: "default"}},
		Roles: []kubesim.Role{
			{Name: "everything", Verbs: []string{"*"}, APIGroups: []string{"*"}, Resources: []string{"*"}},
		},
		RoleBindings: []kubesim.RoleBinding{
			{Name: "oops", RoleKind: "ClusterRole", RoleName: "everything",
				Subjects: []kubesim.BindingSubject{
					{Kind: "ServiceAccount", Namespace: "default", Name: "default"},
				}},
		},
		Pods: []kubesim.Pod{{Namespace: "default", Name: "anything", Phase: corev1.PodRunning}},
	})

	rbac, err := client.AccessControl(ctx, "")
	if err != nil {
		t.Fatalf("AccessControl: %v", err)
	}
	if len(rbac.Accounts) != 1 {
		t.Fatalf("accounts = %+v, want one", rbac.Accounts)
	}
	account := rbac.Accounts[0]
	if !account.Administrative {
		t.Error("the default account is bound to a wildcard role and is not marked administrative")
	}
	if !strings.Contains(account.Notice, "can do anything to the cluster") {
		t.Errorf("notice = %q, want it to say what this means", account.Notice)
	}
	// And what actually runs as it, which is the reason it matters.
	if len(account.UsedBy) != 1 || account.UsedBy[0] != "default/anything" {
		t.Errorf("used by %v, want the pod that names no account", account.UsedBy)
	}
}

// TestAPodThatNamesAnAccountIsAttributedToIt.
func TestAPodThatNamesAnAccountIsAttributedToIt(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := rbacCluster(t)

	rbac, err := client.AccessControl(ctx, "")
	if err != nil {
		t.Fatalf("AccessControl: %v", err)
	}
	for _, account := range rbac.Accounts {
		if account.Namespace != "ci" {
			continue
		}
		if len(account.UsedBy) != 1 || account.UsedBy[0] != "ci/runner-abc" {
			t.Errorf("ci/deployer used by %v, want the pod that names it", account.UsedBy)
		}
		return
	}
	t.Fatal("ci/deployer is not in the answer")
}

// TestABindingWithNoSubjectsGrantsToNobody.
func TestABindingWithNoSubjectsGrantsToNobody(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := rbacCluster(t)

	rbac, err := client.AccessControl(ctx, "")
	if err != nil {
		t.Fatalf("AccessControl: %v", err)
	}
	empty := bindingsByName(rbac)["nobody"]
	if empty.Healthy {
		t.Error("a binding with no subjects is reported as healthy")
	}
	if !strings.Contains(empty.Summary, "to nobody") {
		t.Errorf("summary = %q, want it to say it grants to nobody", empty.Summary)
	}
}

// TestTheBuiltInRolesAreMarked, so a screen can fold seventy identical rows away
// without this having to decide for it.
func TestTheBuiltInRolesAreMarked(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := rbacCluster(t)

	rbac, err := client.AccessControl(ctx, "")
	if err != nil {
		t.Fatalf("AccessControl: %v", err)
	}
	for _, role := range rbac.Roles {
		switch role.Name {
		case "cluster-admin":
			if !role.BuiltIn {
				t.Error("cluster-admin is not marked built in")
			}
		case "platform-operator":
			if role.BuiltIn {
				t.Error("a hand-written role is marked built in")
			}
		}
	}
}

// TestARoleNothingBindsToIsCounted, because a role somebody wrote that no binding
// grants permits nothing at all.
func TestARoleNothingBindsToIsCounted(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := rbacCluster(t)

	rbac, err := client.AccessControl(ctx, "")
	if err != nil {
		t.Fatalf("AccessControl: %v", err)
	}
	for _, role := range rbac.Roles {
		switch role.Name {
		case "platform-operator":
			if role.Bound != 1 {
				t.Errorf("platform-operator is bound %d times, want 1", role.Bound)
			}
		case "configmap-editor":
			if role.Bound != 0 {
				t.Errorf("configmap-editor is bound %d times, want none", role.Bound)
			}
		}
	}
}

// TestAClusterWideGrantIsNotHiddenByANamespaceFilter.
//
// It is the grant that matters most, and a namespace view that dropped it would
// report a namespace as unremarkable while a ClusterRoleBinding gave everything
// in it away.
func TestAClusterWideGrantIsNotHiddenByANamespaceFilter(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := rbacCluster(t)

	rbac, err := client.AccessControl(ctx, "web")
	if err != nil {
		t.Fatalf("AccessControl: %v", err)
	}
	if _, ok := bindingsByName(rbac)["platform-team"]; !ok {
		t.Error("the ClusterRoleBinding is missing from a namespace view")
	}
	if len(rbac.Administrators) == 0 {
		t.Error("a namespace view reports no administrators at all")
	}
	// The namespaced ones ARE narrowed, which is what the filter is for.
	for _, binding := range rbac.Bindings {
		if binding.Kind == "RoleBinding" && binding.Namespace != "web" {
			t.Errorf("%s/%s is in the web-only answer", binding.Namespace, binding.Name)
		}
	}
}
