# Contributing

[English](CONTRIBUTING.md) | [简体中文](CONTRIBUTING.zh.md)

Use Issues to report problems or discuss improvements, and keep Pull Requests focused. Discuss goals and compatibility before making large behavioral or architectural changes. Report security issues privately as described in [SECURITY.md](SECURITY.md).

## Development environment

Follow [development and builds](docs/development.md) to install the mise-pinned toolchain, then run `mise install` and `mise exec -- go run .`. Development and release builds use separate data directories. Integration work must use servers you own or are authorized to access, with isolated test data.

## Submitting changes

- Keep changes focused, reuse existing components and conventions, and update affected user documentation.
- Format Go code with `gofmt`. Follow the [generation workflow](docs/theme-integration.md) for theme tokens; do not edit only generated outputs.
- Use shared language resources for UI text. Run `mise run i18n` and commit both catalogs and the generated file; see [interface languages](docs/i18n.md).
- Keep English and Chinese documents in sync in the same change, including links and validation limits. English is the default `.md`; Chinese uses `.zh.md`. Preserve commands, code identifiers, and original license texts.
- Run existing tests, static checks, and builds appropriate to the change. Check the terminal extension's nested module separately when it is affected. See [existing checks](docs/development.md#existing-checks).
- Explain the actual problem, resulting behavior, validation performed, and environments not covered in your PR. Cross-compilation does not replace native acceptance.
- Do not commit real connection configurations, databases, private keys, passwords, tokens, raw audit records, or unredacted screenshots. Include the version, OS/architecture, reproduction steps, and redacted errors when reporting problems.

## Dependencies and licenses

When adding or updating dependencies, fonts, icons, color schemes, or native libraries, preserve copyright notices and full licenses. Update `internal/desktop/licenses/`, the About page, and `THIRD_PARTY_NOTICES.md`, then run:

```sh
python3 scripts/licenses.py
python3 scripts/licenses.py --check
```

By submitting original contributions, you confirm that you have the right to submit them and agree to license them under this project's [MIT license](LICENSE). Existing third-party material retains its original license.
