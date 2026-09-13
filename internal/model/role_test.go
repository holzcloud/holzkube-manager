package model_test

// The role order is what "at least this role" is decided against, and a role
// inserted in the wrong place widens or narrows every route at once. So the
// order is asserted rather than read off the list.

import (
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/model"
)

func TestRoleOrderIsPrivilegeOrder(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		have model.UserRole
		want model.UserRole
		ok   bool
	}{
		"admin is admin":           {model.RoleAdmin, model.RoleAdmin, true},
		"admin is operator":        {model.RoleAdmin, model.RoleOperator, true},
		"admin is reader":          {model.RoleAdmin, model.RoleReader, true},
		"operator is not admin":    {model.RoleOperator, model.RoleAdmin, false},
		"operator is operator":     {model.RoleOperator, model.RoleOperator, true},
		"operator is reader":       {model.RoleOperator, model.RoleReader, true},
		"reader is not admin":      {model.RoleReader, model.RoleAdmin, false},
		"reader is not operator":   {model.RoleReader, model.RoleOperator, false},
		"reader is reader":         {model.RoleReader, model.RoleReader, true},
		"unset reads as admin":     {"", model.RoleAdmin, true},
		"an unknown role is never": {"superuser", model.RoleReader, false},

		// The other direction of the same rule. A route that asked for a role
		// nobody defined must refuse everybody rather than let everybody
		// through, because the typo is in the route and the people are real.
		"an unknown requirement is never met": {model.RoleAdmin, "superuser", false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := tc.have.AtLeast(tc.want); got != tc.ok {
				t.Errorf("UserRole(%q).AtLeast(%q) = %v, want %v", tc.have, tc.want, got, tc.ok)
			}
		})
	}
}

// TestEveryRoleIsOrdered keeps the table above from silently stopping short of
// a role somebody adds.
func TestEveryRoleIsOrdered(t *testing.T) {
	t.Parallel()

	roles := model.UserRoles()
	if len(roles) != 3 {
		t.Fatalf("there are %d roles and this test knows three; order the new one deliberately "+
			"rather than letting UserRoles decide it", len(roles))
	}

	// Each role must outrank every role after it and no role before it.
	for i, have := range roles {
		for j, want := range roles {
			got := have.AtLeast(want)
			if expect := i <= j; got != expect {
				t.Errorf("%q.AtLeast(%q) = %v, want %v -- UserRoles is ordered most privileged "+
					"first and this pair contradicts that", have, want, got, expect)
			}
		}
		if !have.Valid() {
			t.Errorf("%q is in UserRoles and is not Valid", have)
		}
	}

	if model.UserRole("").Valid() {
		t.Error("the empty role is Valid; it is what a record written before roles existed " +
			"carries, and accepting it as input is how it gets written deliberately")
	}
}
