# Contributing to sandboxMatrix

Thank you for your interest in contributing to sandboxMatrix!

## Development Setup

### Prerequisites

- Go 1.25+
- Docker (for runtime integration tests)
- golangci-lint

### Build

```bash
git clone https://github.com/hg-dendi/sandboxmatrix.git
cd sandboxmatrix
make build
```

### Test

```bash
make test          # Unit tests
make test-race     # With race detector
make lint          # Linting
```

## Pull Request Process

1. Fork the repository and create a feature branch from `main`
2. Write tests for new functionality
3. Ensure `make lint` and `make test` pass
4. Update documentation if needed
5. Submit a PR with a clear description of the changes

## Coding Style

- Follow standard Go conventions (`gofmt`, `goimports`)
- Use meaningful variable and function names
- Keep functions focused and small
- Add comments for non-obvious logic only
- Use table-driven tests where appropriate

## Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add blueprint validation command
fix: handle missing workspace directory
docs: update quickstart guide
test: add runtime registry tests
```

## Reporting Issues

Use GitHub Issues with the provided templates:
- **Bug reports**: Include reproduction steps, expected vs actual behavior
- **Feature requests**: Describe the use case and proposed solution

## Code of Conduct

This project follows the [Contributor Covenant](CODE_OF_CONDUCT.md). Please be respectful and constructive.
