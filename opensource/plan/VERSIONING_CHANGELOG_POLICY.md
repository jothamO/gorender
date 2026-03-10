# Versioning And Changelog Policy (Draft)

## Versioning

Use Semantic Versioning:

- `MAJOR`: breaking CLI/API/contract changes
- `MINOR`: backward-compatible features
- `PATCH`: backward-compatible fixes

## Release Labels

- `alpha`: unstable early public testing
- `beta`: feature-complete with active stabilization
- stable: production-ready release line

## Changelog Rules

Each release must include:

- Added
- Changed
- Fixed
- Deprecated
- Removed

Entries must be user-oriented and include migration notes for breaking changes.

## Compatibility Notes

- Stable flags/commands must remain compatible within a major series.
- Deprecations require at least one minor release warning window before removal.

