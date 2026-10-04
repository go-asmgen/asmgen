# Security policy

## Reporting a vulnerability

Please report a vulnerability **privately**, through GitHub's
[Report a vulnerability](https://github.com/go-asmgen/asmgen/security/advisories/new)
form (private vulnerability reporting is enabled on this repository). Do not
open a public issue for it.

Include the version (`go list -m github.com/go-asmgen/asmgen`), the target
architecture, the builder calls, and the assembly they produced.

## Supported versions

Only the latest release receives fixes. The module follows semantic
versioning; while it is at v0, a fix ships as a patch or minor release.

## What deserves a report

go-asmgen runs at `go generate` time, on input written by the generator's
author, so it has no network-facing surface. Its risk is in what it **emits**:
the assembly it writes runs inside every module that commits it, outside Go's
memory safety. These count as vulnerabilities:

- **An encoding that is not the instruction it names.** Instructions Go's
  assembler does not know yet are emitted as `WORD` directives, which
  `cmd/asm` cannot check (`loong64` LASX/LSX, `ppc64` VSX). Each encoder is
  pinned to reference encodings in `internal/gap` and refuses registers and
  offsets that do not encode. An operand it accepts but encodes wrongly is a
  vulnerability.
- **A frame or argument layout that disagrees with the Go declaration** in a
  way that `go vet` (asmdecl) does not catch, so the function reads or writes
  memory it does not own.
- **A builder that emits something other than what its documentation
  promises**: a different register, width or offset, or a clobbered
  callee-saved register.

Not covered: text passed to `Raw` and the other escape hatches. It is emitted
as written, and checking it is the caller's job, through `go vet` and
`cmd/asm`, as for hand-written assembly.

## Audit of 2026-10-04 (v0.15.0)

- **govulncheck:** no vulnerability. The module has no dependencies. CI now runs
  govulncheck on every change.
- **gosec and staticcheck:** gosec flags integer conversions: in the encoders,
  each one is preceded by a range check (`reg`, `ldrepl`); in `Hex` and in the
  wasm printer, the truncation is the intent (bytes of a word, a byte printed
  as a signed `i8`). It also flags the `go` subprocesses
  and file writes of the tests and of `tools/goasmgap`, which exist to drive
  the assembler. No finding needed a change. staticcheck reports only
  `runtime.GOROOT` in a test (deprecated, not a risk).
- **Encoders:** the tests cover every rejection (register 32, offset not a
  multiple of 8, offset out of range).
- **CI:**
  - the workflow token is read-only, except `issues: write` in the scheduled
    gap report, which never runs on a pull request;
  - workflows run on `pull_request`, not `pull_request_target`;
  - no secrets are used, and no event data is interpolated into a shell
    command.
- **Repository:** private vulnerability reporting, secret scanning and push
  protection are enabled, on this repository and on the organisation's
  others.
