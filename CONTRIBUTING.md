# Contributing to gowhatsapp

Thanks for your interest in contributing!

## Getting Started

1. Fork the repository
2. Clone your fork: `git clone https://github.com/<username>/gowhatsapp.git`
3. Create a branch: `git checkout -b my-feature`
4. Make your changes
5. Run checks: `make ci`
6. Push and open a pull request

## Development

### Prerequisites

- Go 1.22+
- golangci-lint v2

### Common tasks

```bash
make test        # run tests
make test-race   # run tests with the race detector
make coverage    # coverage summary
make lint        # run linter
make ci          # fmt-check, vet, lint, race tests (the CI gate)
```

### Code Style

- Standard Go conventions; run `gofmt -s` and `goimports` before committing.
- All exported types and functions must have doc comments.
- The library is **dependency-free** — do not add third-party imports to the
  core without discussion.
- New message types implement the sealed `Message` interface; new endpoints go
  through the `Doer` transport seam. See [`DESIGN.md`](./DESIGN.md).
- Keep coverage high; payload-shape changes need a wire-shape test.

## Pull Requests

- Keep PRs focused on a single change.
- Include tests for new functionality.
- Update `README.md` / `doc.go` / `CHANGELOG.md` when the public API changes.
- Ensure `make ci` passes before requesting review.

## Reporting Issues

- Use GitHub Issues.
- Include Go version, OS, a minimal reproduction, and any `fbtrace_id` from an
  `*APIError` (never paste access tokens or app secrets).

## License

By contributing you agree that your contributions will be licensed under the
MIT License.
