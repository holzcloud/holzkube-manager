package handlers

import "github.com/holzcloud/holzkube/internal/httpapi"

// AccountRoutes is deliberately empty in this plan.
//
// It exists now so that plan 04 adds the password change as a route in this
// file -- marked Destructive: true, because changing the credential that guards
// cluster PKI is exactly what the sudo gate is for -- without touching the
// composition root or the router.
func AccountRoutes(_ httpapi.Deps) []httpapi.Route {
	return nil
}
