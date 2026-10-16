// Command rpcgen generates the notesd RPC contract from schema/rpc/*.yaml:

// the Go half of package api, the TypeScript client for the app, and the
// API reference. `make gen` writes the outputs; `make gen-check` (part of
// make verify) fails when any output is stale.
//
//	go run ./tools/rpcgen [-check] [-schema DIR] [-go FILE] [-ts FILE] [-md FILE]
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "rpcgen:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("rpcgen", flag.ContinueOnError)
	check := fs.Bool("check", false, "fail if any output differs from what would be generated")
	schemaDir := fs.String("schema", "schema/rpc", "IDL directory")
	goOut := fs.String("go", "api/zz_generated.go", "Go output")
	tsOut := fs.String("ts", "app/src/rpc/gen/api.ts", "TypeScript output")
	mdOut := fs.String("md", "docs/specs/rpc-api.md", "Markdown output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	s, err := Load(*schemaDir)
	if err != nil {
		return err
	}
	goSrc, err := genGo(s)
	if err != nil {
		return err
	}
	outputs := []struct {
		path string
		data []byte
	}{
		{*goOut, goSrc},
		{*tsOut, genTS(s)},
		{*mdOut, genMarkdown(s)},
	}
	var stale []string
	for _, o := range outputs {
		if *check {
			cur, err := os.ReadFile(o.path)
			if err != nil || !bytes.Equal(cur, o.data) {
				stale = append(stale, o.path)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(o.path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(o.path, o.data, 0o644); err != nil {
			return err
		}
	}
	if len(stale) > 0 {
		return fmt.Errorf("generated files are stale, run make gen: %v", stale)
	}
	return nil
}
