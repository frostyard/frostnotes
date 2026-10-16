// Command vaultgen writes a fictional vault and an optional link manifest.
//
//	go run ./tools/vaultgen -out DIR [-notes 666] [-seed 1] [-manifest FILE]
package main

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/frostyard/frostnotes/internal/vaultgen"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "vaultgen:", err)
		os.Exit(2)
	}
}

func run(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("vaultgen", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	out := fs.String("out", "", "the folder to write (required)")
	notes := fs.Int("notes", 666, "how many notes")
	seed := fs.Uint64("seed", 1, "the seed")
	manifest := fs.String("manifest", "", "write the manifest to this file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %q", fs.Args())
	}
	if *out == "" {
		return fmt.Errorf("-out is required")
	}
	m, err := vaultgen.Generate(*out, vaultgen.Options{Seed: *seed, Notes: *notes})
	if err != nil {
		return err
	}
	if *manifest != "" {
		data, err := json.Marshal(m, jsontext.WithIndent("  "))
		if err != nil {
			return fmt.Errorf("encode manifest: %w", err)
		}
		if err := os.WriteFile(*manifest, append(data, '\n'), 0o644); err != nil {
			return fmt.Errorf("write manifest: %w", err)
		}
	}
	_, err = fmt.Fprintf(stdout, "wrote %d notes, %d attachments and %d links to %s\n",
		len(m.Notes), len(m.Attachments), len(m.Links), *out)
	return err
}
