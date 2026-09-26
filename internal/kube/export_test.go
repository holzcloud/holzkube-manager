package kube

import "time"

// SetSummaryBudget shortens the shared deadline on the kubelet reads for one
// test, and returns what puts it back.
//
// It exists so a hanging kubelet can be proven cut off in a fraction of a
// second instead of in the ten the product allows. A test that calls it must
// not be parallel: the budget is package state, and Go releases parallel tests
// only after every sequential one has finished, which is what makes changing it
// from a sequential test safe.
func SetSummaryBudget(d time.Duration) (restore func()) {
	previous := summaryBudget
	summaryBudget = d
	return func() { summaryBudget = previous }
}
