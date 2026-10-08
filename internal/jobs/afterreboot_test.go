package jobs

import (
	"context"
	"errors"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/model"
)

func TestAfterRebootedRunsTheHookOnlyWhenTheNodeRebooted(t *testing.T) {
	t.Parallel()

	var cleared []model.MachineID
	hook := func(id model.MachineID) { cleared = append(cleared, id) }

	failing := afterRebooted(Step{
		Do:       func(context.Context, *model.Job) error { return errors.New("refused") },
		Happened: func(context.Context, *model.Job) (bool, error) { return false, nil },
	}, "m1", hook)
	if err := failing.Do(t.Context(), &model.Job{}); err == nil {
		t.Fatal("the wrapped step swallowed its error")
	}
	if ok, _ := failing.Happened(t.Context(), &model.Job{}); ok || len(cleared) != 0 {
		t.Fatalf("hook ran for a reboot that did not happen: %v", cleared)
	}

	good := afterRebooted(Step{
		Do:       func(context.Context, *model.Job) error { return nil },
		Happened: func(context.Context, *model.Job) (bool, error) { return true, nil },
	}, "m2", hook)
	if err := good.Do(t.Context(), &model.Job{}); err != nil {
		t.Fatal(err)
	}
	if len(cleared) != 1 || cleared[0] != "m2" {
		t.Fatalf("hook after Do: %v", cleared)
	}
	// An interrupted reboot found already done on resume counts as well.
	if ok, _ := good.Happened(t.Context(), &model.Job{}); !ok || len(cleared) != 2 {
		t.Fatalf("hook after a resumed, already-happened reboot: %v", cleared)
	}
}

func TestAfterRebootedWithoutAHookIsTheStepItself(t *testing.T) {
	t.Parallel()

	s := Step{Name: "x", Do: func(context.Context, *model.Job) error { return nil }}
	if got := afterRebooted(s, "m", nil); got.Happened != nil || got.Name != "x" {
		t.Fatal("a nil hook changed the step")
	}
}
