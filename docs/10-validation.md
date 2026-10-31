# Planning artifact validation

Validation scope: schemas, examples, OpenAPI structure, local document links, and selected cross-field invariants. This repository does not contain application code or deployed infrastructure.

## Reproduce

With Python 3.11+ and pip available:

```sh
python3 -m venv .venv
.venv/bin/python -m pip install -r requirements-specs.txt
.venv/bin/python scripts/check_specs.py
git diff --check
```

The optional dependencies are pinned in [requirements-specs.txt](../requirements-specs.txt). The checker is [scripts/check_specs.py](../scripts/check_specs.py); it does not make network requests during validation. Tool installation may require network access.

## Checks

- Draft 2020-12 schema validity for schedule and manifest.
- Format-aware validation of four synthetic examples, including the 168-entry, seven-day manifest.
- OpenAPI 3.1 validity with local external schema references.
- Selected semantic checks: real timezone, UTC entry order, coverage, inventory references, dimensions, schedule/manifest revision and collection-reference agreement, and fixture cache budget.
- Negative fixtures for unsupported frequency, duplicate selected folder, removed rule-builder fields, invalid checksum, duplicate timestamp, and missing variant.
- Existence of local Markdown link targets and Git whitespace checks.

The collection-first revision passed on **2026-10-31** using Python 3.12, jsonschema 4.26.0, openapi-spec-validator 0.9.0, and PyYAML 6.0.3. Result: two schemas, OpenAPI 3.1, four examples, selected domain invariants, six negative fixtures, and 34 local Markdown links passed. Git whitespace checks also passed. Validators were installed into a temporary tooling directory, not into the application workspace.

This checker is not the future schedule compiler and does not prove its DST behavior, authorization, source compliance, offline reliability, performance, or operating-system compatibility. Those require the implementation and [acceptance scenarios](../specs/06-acceptance-scenarios.md).
