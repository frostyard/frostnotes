# Executor rules

You are implementing one task card. The card is the whole job.

1. Read the card, then every file under "Read first", before writing code.
2. Change only the files listed under `touch` in the card's front matter.
   Create them if they do not exist. Never create, edit, rename or delete any
   other file.
3. Never edit the given test files. They define the contract. If a test seems
   wrong, stop and explain why in your final message instead of changing it.
4. Never edit `go.mod`, `go.sum`, `package.json`, `pnpm-lock.yaml`,
   anything under `schema/`, `api/zz_generated.go`, `app/src/rpc/gen/`, a
   migration under `internal/store/migrations/`, or anything under
   `internal/merge/`. Use only the dependencies already in the module.
5. Run commands through make: `make accept T=NNNN` runs the card's tests,
   `make check` runs formatting, vet, lint and every unit test. Both must pass.
   For a card that touches `app/`, `make ui-check` (Biome lint and format
   check, the TypeScript check, every Vitest test) must pass too;
   `make ui-fmt` formats your files. Never run `pnpm`, `npx` or `node`
   directly. If your shell lacks the pinned tools (Codex's does), run make
   through mise: `mise exec -- make check`. If the app's targets cannot
   reach the nsl machine (Codex's sandbox cannot), do not run them; taskrun
   formats your files, runs the make targets after you finish, and sends
   you any failure.
6. Fix the code, not the check: no `//nolint`, no skipped tests, no deleted
   assertions.
7. After five failed attempts at the same failure, stop and write what you
   tried and what you think is wrong in your final message.
8. Never name a unit test `TestI…` (`TestIndex…`, `TestInit…`): `make test`
   skips them (frostyard/core ADR-0022) and `internal/repocheck` fails.

## Notes are the user's files

- A note is written byte for byte: never normalize line endings, trailing
  newlines, BOMs or frontmatter formatting (ADR-0003).
- Every write names the version it changes; never write a note without its
  base (ADR-0006). The card says which helper does this.
- Test vaults are generated or hand-written in temporary directories. Never
  read or write a real vault.

## Go conventions

- `context.Context` is the first parameter of anything that does I/O.
- Wrap errors with context: `fmt.Errorf("insert vault: %w", err)`. Error
  strings are lowercase with no trailing punctuation.
- No mutable package-level variables. Compiled regexps and constant tables at
  package level are fine.
- Every exported identifier has a doc comment that starts with its name.
- Prefer the standard library and the helpers the card names over new code.
- Keep functions under 60 lines.
- `encoding/json/v2`; `slog` for logs; tests use `t.TempDir()`, `t.Context()`
  and `rpctest`, and never sleep to synchronize.

## TypeScript conventions

- Strict mode, no `any`, no non-null assertions (`!`).
- Call notesd only through the generated client in `app/src/rpc/gen/api.ts`.
- Never use `dangerouslySetInnerHTML`.
- Components in `app/src/features/` are presentational: they get data and
  callbacks through props and import nothing from `app/src/data/` or
  `app/src/rpc/` except types from `rpc/gen/api.ts`.
- Style with Tailwind utilities over the theme in `app/src/styles/app.css`.
  Never write literal colors (`#fff`, `rgb(…)`, `text-gray-500`).
- Icons come from `lucide-react`; give icon-only controls an `aria-label`.
- Function components with named exports; no default exports, no classes.
