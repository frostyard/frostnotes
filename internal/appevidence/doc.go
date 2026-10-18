// Package appevidence holds M3's exit evidence 2 and 3
// (docs/plans/0005-m3-desktop-app.md, Phase 7): integration tests that run
// the built app (FROSTNOTES_APP) under X, press its keys with xdotool, and
// write its open note from outside. make m3-evidence runs them under
// Xvfb in the nsl machine; the page's half is app/src/spike/drive.tsx.
package appevidence
