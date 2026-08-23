# OpenClaw Tool Compatibility

**Date**: 2026-08-22
**Status**: In Progress

## Goal

Ensure DocsClaw's OpenClaw-compatible tool set works reliably in shipped
container images, with clear runtime errors when a required external binary
is missing.

## search_files

The `search_files` tool shells out to [ripgrep](https://github.com/BurntSushi/ripgrep)
(`rg`) to search file contents. At execution time it checks for `rg` on
`PATH` using `exec.LookPath`; if ripgrep is not installed, the tool returns
the error:

```
ripgrep (rg) is not installed; search_files requires it
```

All shipped container images install the `ripgrep` package so the tool
works at runtime:

- `Containerfile`
- `Dockerfile`
- `Dockerfile.release`
- `containers/Containerfile.security`
