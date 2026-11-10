package editor

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostnotes/internal/desktop/notes"
)

// NoticeActions are what a tab's notice leads to, which the window wires.
type NoticeActions struct {
	// CompareVersion opens the merge view of the note against one of its
	// versions: the text a conflict kept.
	CompareVersion func(version int64)
	// CompareText opens the merge view of the note against text: what the
	// editor held when notesd refused its base.
	CompareText func(text string)
	// Close closes the tab of a deleted note.
	Close func()
}

// Notice draws what a note's tabs say about it
// (docs/specs/ui.md#notices-in-a-tab) as frostui's banner, and runs what
// the user chooses: a conflict (Compare, Dismiss), a refused base
// (Compare), a deleted note (Restore, and close for the tab), a failed
// save (Retry now), and a note that cannot be edited here. It draws
// nothing when the note has nothing to say.
func Notice(c *ui.Context, d *notes.Document, a NoticeActions) {
	if reason := d.ReadOnly(); reason != "" {
		frostui.Banner(c, frostui.Warning, "dialog-information", reason, "", false)
		return
	}
	n := d.Notice()
	if n == nil {
		return
	}
	switch n.Kind {
	case notes.NoticeConflict:
		changes := "1 change"
		if n.Conflicts != 1 {
			changes = fmt.Sprintf("%d changes", n.Conflicts)
		}
		text := changes + " conflicted with " + n.By + "; your text is kept as a version."
		compare, dismiss := frostui.Banner(c, frostui.Warning, "dialog-warning", text, "Compare", true)
		if compare && a.CompareVersion != nil && n.Version != 0 {
			a.CompareVersion(n.Version)
		}
		if dismiss {
			d.Dismiss()
		}
	case notes.NoticeRefused:
		text := "Not saved: notesd no longer has the version this text started from. Your text is kept."
		if compare, _ := frostui.Banner(c, frostui.Danger, "dialog-error", text, "Compare", false); compare && a.CompareText != nil {
			a.CompareText(n.Text)
		}
	case notes.NoticeDeleted:
		closeLabel := ""
		if a.Close != nil {
			closeLabel = "Close tab"
		}
		restore, closeTab := frostui.BannerClose(c, frostui.Neutral, "user-trash", "This note was deleted.", "Restore", closeLabel)
		if restore {
			d.Restore()
		}
		if closeTab {
			a.Close()
		}
	case notes.NoticeFailed:
		if retry, _ := frostui.Banner(c, frostui.Danger, "dialog-error", "Not saved: "+n.Reason+". Retrying.", "Retry now", false); retry {
			d.RetryNow()
		}
	}
}
