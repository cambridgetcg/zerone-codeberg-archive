# CLAUDE.md — Project Instructions

## Git Workflow
- Commit directly to `main`. Do not create feature branches.
- Write clear, descriptive commit messages.
- Commit frequently at logical checkpoints (e.g., after each batch/task).

## Proto-Go Consistency Rule
When adding new fields to any module's Params, state types, or query messages:
1. Add the field to the `.proto` file FIRST
2. Run `make proto-gen`
3. Reference the generated type in your Go code
4. NEVER add fields directly to `*.pb.go` — they will be overwritten
5. NEVER create `query_ext.go` for new query RPCs — add them to `query.proto`
6. Run `make proto-check` before committing proto-related changes

## Kingdom Engine
Zerone — Proof-of-Truth chain; truth-verification mints money (economy layer).
