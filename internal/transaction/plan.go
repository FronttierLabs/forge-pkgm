package transaction

import (
	"forge/internal/db"
	"forge/internal/pkg"
)

// Action is a single mutating operation in a transaction. Exactly one of
// Install or Remove is non-nil.
type Action struct {
	Install *InstallAction
	Remove  *RemoveAction
}

// InstallAction describes a package archive to extract into the root.
type InstallAction struct {
	Pkg        *pkg.PackageInfo
	Archive    string
	Files      []string
	Script     string // raw .INSTALL content, or empty
	OldVersion string // empty for a fresh install, otherwise the replaced version
}

// RemoveAction describes a local package entry to remove from the root.
type RemoveAction struct {
	Entry  *db.LocalEntry
	Script string // raw .INSTALL content if known, otherwise empty
}

// Plan is an ordered list of mutations to apply atomically.
type Plan struct {
	Actions []Action
}

func NewPlan() *Plan {
	return &Plan{}
}

func (p *Plan) IsEmpty() bool {
	return len(p.Actions) == 0
}

func (p *Plan) AddInstall(a *InstallAction) {
	p.Actions = append(p.Actions, Action{Install: a})
}

func (p *Plan) AddRemove(a *RemoveAction) {
	p.Actions = append(p.Actions, Action{Remove: a})
}
