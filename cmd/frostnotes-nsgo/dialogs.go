package main

import (
	"github.com/frostyard/frostui/workspaceui"
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// The app's dialogs (the kit's standard for an app, rules 3 and 4). A form
// is NSGo's dialog and a confirmation its alert dialog, both on the kit's
// panel. Cancel is quiet and comes first, the confirming button is the one
// filled button, and a destroying action is the danger variant. What went
// wrong shows in the dialog as a banner in the danger tone.

// form is a dialog with fields. busy holds it open and dims its buttons
// while its action runs; ready, when set, dims the confirming button until
// the fields can be sent.
type form struct {
	title, detail string
	fields        []core.Node
	problem       func() string
	busy          func() bool
	ready         func() bool
	confirm       string
	submit        func()
}

func (w *workspace) form(cx *core.Scope, f form) core.Node {
	busy := func() bool { return f.busy != nil && f.busy() }
	children := append(append([]core.Node{}, f.fields...), failure(f.problem))
	return ui.Dialog{Title: f.title, Description: f.detail, StyleFunc: workspaceui.DialogPanel(cx), Open: func() bool { return true },
		OnOpenChange: func(open bool, r base.Reason) {
			if f.busy == nil || !core.Untracked(cx, f.busy) {
				w.closeDialog(open, r)
			}
		},
		Children: children, Footer: []core.Node{
			ui.Button{Text: "Cancel", Variant: ui.ButtonSecondary, Disabled: busy, OnPress: func() { w.action.Set("") }},
			ui.Button{Text: f.confirm, Disabled: func() bool { return busy() || f.ready != nil && !f.ready() }, OnPress: f.submit}}}
}

// failure is what went wrong, as a banner, and nothing while problem is
// empty.
func failure(problem func() string) core.Node {
	return core.Dynamic(func(*core.Scope) core.Node {
		if text := problem(); text != "" {
			return workspaceui.Banner{Tone: workspaceui.Danger, Icon: lucide.CircleAlert, Text: text}
		}
		return nil
	})
}

// destroy asks before an action that removes something: the alert dialog,
// with Cancel first and the action in the danger variant. The alert closes
// as the action starts, so what the action then fails at shows over the
// window.
func (w *workspace) destroy(cx *core.Scope, title, detail, label string, do func()) core.Node {
	return ui.AlertDialog{Title: title, Description: detail, StyleFunc: workspaceui.DialogPanel(cx), Open: func() bool { return true },
		OnOpenChange: w.closeDialog,
		Actions: []ui.AlertAction{
			{Text: "Cancel", Variant: ui.ButtonSecondary, Role: core.AlertCancel},
			{Text: label, Variant: ui.ButtonDanger, Default: true, OnPress: func() { w.action.Set(""); do() }}}}
}

// field is a form's text field.
func field(label, placeholder string, s *core.Signal[string], busy func() bool, submit func()) ui.Input {
	in := ui.Input{Label: label, Placeholder: placeholder, Value: s.Get, OnChange: s.Set, Disabled: busy}
	if submit != nil {
		in.OnSubmit = func(string) { submit() }
	}
	return in
}
