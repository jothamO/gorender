# Release Automation Plan (Draft)

## Goal

Automate tagged releases with reproducible binaries and checksums while preserving lightweight constraints.

## Draft Workflow

1. Tag created: `vX.Y.Z`
2. CI validates:
   - tests
   - build matrix
3. Build binaries:
   - Windows
   - Linux
   - macOS
4. Generate checksums (SHA256).
5. Publish GitHub Release notes from changelog.

## Required Inputs

- signed/approved tag
- finalized changelog entry
- passing CI status

## Artifacts

- `gorender` binaries per platform
- `gorendersd` binaries per platform
- checksums file

## Future Enhancements

- SBOM generation
- provenance attestations
- optional container images

