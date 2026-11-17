// Package query evaluates the subset of Obsidian Bases that notesd
// supports (docs/specs/bases.md, ADR-0037): it reads a base's YAML,
// checks it against the subset, folds constants, and compiles a view's
// filters, sort and groups to SQL over the index's files and notes tables.
// store.DB runs the SQL; the engine serves it as note.query.
//
// Every property key and literal of a base reaches SQL as a bound
// parameter, never as SQL text; Compile writes only fixed text. The
// package keeps no state between calls.
package query
