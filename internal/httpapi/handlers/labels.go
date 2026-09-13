package handlers

// Machine labels and machine classes (Omni parity phase 5).
//
// Labels are operator writes and belong to an operator; classes are the same.
// Neither is Destructive: a label changes nothing on any node, and a class is a
// question this installation keeps about its own inventory. What a class is
// *used for* — a cluster definition, later — is where the destructive marking
// belongs, and putting it here instead would train an operator to type their
// password to rename a group.

import (
	"errors"
	"net/http"

	"github.com/holzcloud/holzkube-manager/internal/httpapi"
	"github.com/holzcloud/holzkube-manager/internal/inventory"
	"github.com/holzcloud/holzkube-manager/internal/model"
)

// LabelRoutes serves the labels on a machine and the classes over them.
func LabelRoutes(d httpapi.Deps) []httpapi.Route {
	return []httpapi.Route{
		{
			Method:          http.MethodPut,
			Pattern:         "/api/v1/machines/{id}/labels",
			RequiresSession: true,
			MinRole:         model.RoleOperator,
			Action:          "machine.labels",
			Handler:         handler(setMachineLabels(d)),
		},
		{
			Method:          http.MethodGet,
			Pattern:         "/api/v1/machine-classes",
			RequiresSession: true,
			MinRole:         model.RoleReader,
			Handler:         handler(listMachineClasses(d)),
		},
		{
			Method:          http.MethodPut,
			Pattern:         "/api/v1/machine-classes/{id}",
			RequiresSession: true,
			MinRole:         model.RoleOperator,
			Action:          "machine-class.put",
			Handler:         handler(putMachineClass(d)),
		},
		{
			Method:          http.MethodDelete,
			Pattern:         "/api/v1/machine-classes/{id}",
			RequiresSession: true,
			MinRole:         model.RoleOperator,
			Action:          "machine-class.delete",
			Handler:         handler(deleteMachineClass(d)),
		},
	}
}

// machineClassView is a class plus the answer to the only question anybody
// asks about one: which machines is this, right now.
//
// The membership is computed on every read rather than stored, because a class
// is a question and not a group. Storing it would mean a second thing to keep
// in step with the labels, and the moment those two disagree is the moment an
// operator stops trusting either.
type machineClassView struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	Description string              `json:"description,omitempty"`
	Selector    model.LabelSelector `json:"selector"`

	// Sentence is Selector in words. The data is there too; this is what
	// somebody actually reads, in the same way the etcd member list carries
	// one.
	Sentence string `json:"sentence"`

	// Machines are the ids this class currently names, and Count is how many.
	// A class that names nothing is the interesting case and reads as an empty
	// list rather than as a missing field.
	Machines []model.MachineID `json:"machines"`
	Count    int               `json:"count"`
}

func setMachineLabels(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := inventoryConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}

		var body struct {
			Labels map[string]string `json:"labels"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			httpapi.WriteProblem(w, r, decodeProblem(err))
			return
		}

		view, err := d.Inventory.SetLabels(r.Context(), model.MachineID(r.PathValue("id")), body.Labels)
		if err != nil {
			writeLabelError(w, r, d, err)
			return
		}
		writeJSON(w, http.StatusOK, view)
	}
}

func listMachineClasses(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := inventoryConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}

		classes, err := d.Inventory.MachineClasses(r.Context())
		if err != nil {
			writeInventoryError(w, r, d, err)
			return
		}

		views := make([]machineClassView, 0, len(classes))
		for _, c := range classes {
			view, err := viewOfClass(r, d, c)
			if err != nil {
				writeInventoryError(w, r, d, err)
				return
			}
			views = append(views, view)
		}
		writeJSON(w, http.StatusOK, map[string]any{"classes": views})
	}
}

func putMachineClass(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := inventoryConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}

		var body struct {
			Name        string              `json:"name"`
			Description string              `json:"description"`
			Selector    model.LabelSelector `json:"selector"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			httpapi.WriteProblem(w, r, decodeProblem(err))
			return
		}

		saved, err := d.Inventory.PutMachineClass(r.Context(), model.MachineClass{
			ID:          model.MachineClassID(r.PathValue("id")),
			Name:        body.Name,
			Description: body.Description,
			Selector:    body.Selector,
		})
		if err != nil {
			writeLabelError(w, r, d, err)
			return
		}

		view, err := viewOfClass(r, d, saved)
		if err != nil {
			writeInventoryError(w, r, d, err)
			return
		}
		writeJSON(w, http.StatusOK, view)
	}
}

func deleteMachineClass(d httpapi.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p := inventoryConfigured(d); p != nil {
			httpapi.WriteProblem(w, r, p)
			return
		}

		err := d.Inventory.DeleteMachineClass(r.Context(), model.MachineClassID(r.PathValue("id")))
		if err != nil {
			writeLabelError(w, r, d, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func viewOfClass(r *http.Request, d httpapi.Deps, c model.MachineClass) (machineClassView, error) {
	matched, err := d.Inventory.MachinesMatching(r.Context(), c.Selector)
	if err != nil {
		return machineClassView{}, err
	}

	ids := make([]model.MachineID, 0, len(matched))
	for _, m := range matched {
		ids = append(ids, m.ID)
	}

	return machineClassView{
		ID:          string(c.ID),
		Name:        c.Name,
		Description: c.Description,
		Selector:    c.Selector,
		Sentence:    c.Selector.Sentence(),
		Machines:    ids,
		Count:       len(ids),
	}, nil
}

func writeLabelError(w http.ResponseWriter, r *http.Request, d httpapi.Deps, err error) {
	switch {
	case errors.Is(err, inventory.ErrInvalidLabels):
		// Every reason at once, in the detail. They are joined rather than
		// turned into FieldErrors because the field they are about is one map:
		// "labels" as a path says nothing, and "labels.rack" would claim a
		// structure the request body does not have.
		httpapi.WriteProblem(w, r, httpapi.Validation(err.Error()))
	case errors.Is(err, inventory.ErrSelectorEmpty):
		httpapi.WriteProblem(w, r, httpapi.Validation(
			"A machine class with no conditions matches no machines, and storing one is almost "+
				"always a selector that was cleared by accident. Name at least one condition.",
			httpapi.FieldError{Field: "selector", Reason: "needs at least one condition"}))
	default:
		writeInventoryError(w, r, d, err)
	}
}
