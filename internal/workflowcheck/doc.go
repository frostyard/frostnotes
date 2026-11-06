// Package workflowcheck holds the tests that keep the repository's own
// files in line with frostyard/core: the GitHub Actions workflows within
// ADR-0021 (external actions pinned to a full commit SHA, permissions
// declared at the top, and checkouts that do not keep the token), and the
// files of the repository baseline (ADR-0071, rows B1, B2, B6 and B13): the
// instruction symlinks, the corrections inbox, the risk-path list and the
// Dependabot entries.
package workflowcheck
