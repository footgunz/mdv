# Rename mdthing → mdv

Full rename of the tool: module path, binary, CLI strings, config path,
build files, and living docs.

## Goals

- Module path `github.com/dgunther/mdv`; the one internal import
  (`…/mermaid` in render.go) updated; `go install` produces a binary
  named `mdv`.
- CLI surface: usage line `usage: mdv [-html] [-mermaid-renderer
  native|js] <file.md>`; every stderr prefix `mdthing:` → `mdv:`
  (including `mdv: config:` warnings).
- Config path: `$XDG_CONFIG_HOME/mdv/config` (default
  `~/.config/mdv/config`). Clean break, no legacy fallback — verified no
  real `~/.config/mdthing/` exists on this machine.
- Taskfile: `build` outputs `mdv`, `clean` removes it; `.gitignore`
  ignores `/mdv` (drop `/mdthing`).
- Docs: README fully renamed; `examples/config` and `examples/user.css`
  comment paths updated.
- Cleanup: stale `~/go/bin/mdthing` and the repo-local `mdthing` build
  artifact removed after `mdv` installs.

## Non-goals

- Renaming the project directory (`~/Projects/mdthing`) — user's call,
  nothing depends on it.
- Editing historical specs/plans under `docs/superpowers/` — they are
  records of what was done under the old name.
- Any behavior change beyond the config path.

## Verification

- `go build ./... && go vet ./... && go test ./... -race` green.
- `grep -rn mdthing` clean outside `docs/superpowers/` and `.git`.
- `task install` produces `~/go/bin/mdv`; smoke: `mdv -html` on a sample
  renders; viewer opens/exits; bad flag prints the new usage.
- Config read from `~/.config/mdv/config` (verified via XDG override in
  the existing test + a live smoke with a scratch XDG dir).
