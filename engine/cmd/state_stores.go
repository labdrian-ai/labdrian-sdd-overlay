package main

import (
	"fmt"
	"os"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/gitprov"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/projection/fsstore"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewreceipt"
	receiptfs "github.com/labdrian-ai/labdrian-sdd-overlay/engine/reviewreceipt/fsstore"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/roles"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/roles/filechain"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/shaper"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/shaper/fsadapter"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/statestore"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/workflow/filelog"
)

// This file is where the file-backed stores of the domain are built: the workflow
// event log, the role handoff chain, the session binding store and the shaper's
// clearance store. Each adapter is handed the state home and reads no environment
// variable, so the composition root is the one place that learns where the state lives
// ($XDG_STATE_HOME, or $HOME/.local/state; see statestore.Home). A home that cannot be
// resolved is reported in the words each store has always used for it, with the store's
// name in front. The shaper's contained source is built here too: it keeps no state, so it
// has no home to be handed. So is the review receipt capture, which keeps its state in the
// project it is started for, not in the state home: it is handed the project root and the
// git it asks through, whose ambient environment is read here and nowhere else.

// resolveStateHome resolves the state home from the environment for the store named
// store, which prefixes the error.
func resolveStateHome(store string) (string, error) {
	stateHome, err := statestore.Home()
	if err != nil {
		return "", fmt.Errorf("%s: %w", store, err)
	}
	return stateHome, nil
}

// newWorkflowStore builds the workflow's event log: the file-backed adapter over the
// state home.
func newWorkflowStore() (workflow.EventLog, error) {
	stateHome, err := resolveStateHome("workflow store")
	if err != nil {
		return nil, err
	}
	store, err := filelog.NewStore(stateHome)
	if err != nil {
		return nil, err
	}
	return store, nil
}

// newRoleChainStore builds the role handoff chain log: the file-backed adapter over
// the state home.
func newRoleChainStore() (roles.ChainLog, error) {
	stateHome, err := resolveStateHome("role chain store")
	if err != nil {
		return nil, err
	}
	store, err := filechain.NewStore(stateHome)
	if err != nil {
		return nil, err
	}
	return store, nil
}

// newBindingStore builds the binding store of the projection domain: the file-backed
// adapter over the state home.
func newBindingStore() (projection.BindingStore, error) {
	stateHome, err := resolveStateHome("projection store")
	if err != nil {
		return nil, err
	}
	store, err := fsstore.NewStore(stateHome)
	if err != nil {
		return nil, err
	}
	return store, nil
}

// newClearanceStore builds the shaper's clearance store: the file-backed adapter over the
// state home.
func newClearanceStore() (shaper.ClearanceStore, error) {
	stateHome, err := resolveStateHome("clearance store")
	if err != nil {
		return nil, err
	}
	store, err := fsadapter.NewClearanceStore(stateHome)
	if err != nil {
		return nil, err
	}
	return store, nil
}

// newContainedSource builds the shaper's source of the handoff and Goal files: the
// file-backed adapter, with the production behavior of the read (no test hook).
func newContainedSource() shaper.ContainedSource {
	return fsadapter.ContainedSource{}
}

// newReviewReceiptService builds the review receipt capture for the project at root, which
// is the directory the verb was given: the file-backed adapter, finding the review
// transaction stores through gitprov with the real git binary and the process environment.
func newReviewReceiptService(root string) (*reviewreceipt.Service, error) {
	store, err := receiptfs.New(root, gitprov.Observer{Run: gitprov.ExecRunner, Environ: os.Environ()})
	if err != nil {
		return nil, err
	}
	return reviewreceipt.NewService(reviewreceipt.Ports{Stores: store, Source: store, Sink: store, Changes: store}), nil
}
