# Changelog

All notable changes to Smithy will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

# 1. Update CHANGELOG
cat >> CHANGELOG.md <<EOF

## [1.0.4] - 2025-10-12


### Fixed
- GHCR authentication in release pipeline
- Version tag format (removed 'v' prefix)
- Multi-arch build workflow

EOF

# 2. Commit
git add CHANGELOG.md
git commit -m "Release v1.0.4"

# 3. Create and push new tag
git tag v1.0.4
git push origin main
git push origin v1.0.4


### Added
- New features here

### Changed
- Changes in existing functionality

### Fixed
- Bug fixes

## [1.0.2] - 2025-10-12

### Added
- Multi-arch support (amd64, arm64)
- Comprehensive documentation
- GitHub Actions workflow

### Fixed
- Cache permission issues
- Git authentication in builds

## [1.0.1] - 2025-10-11

### Added
- Initial public release
- Basic build functionality

