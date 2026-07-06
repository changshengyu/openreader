---
name: readerdev-compat-inventory
description: Compatibility inventory workflow for OpenReader vs reader-dev. Use before implementing or changing any OpenReader feature that exists in upstream reader-dev; this skill analyzes and documents differences without editing code.
---

# Reader-dev Compatibility Inventory

Use this skill before code changes for upstream-backed features.

## Hard rule

Do not edit application code while using this skill. Produce or update a compatibility matrix first.

## Workflow

1. Locate the reader-dev source files for the feature.
2. Locate the current OpenReader files for the same feature.
3. Extract behavior, not just component names:
   - route and query parameters;
   - API method/path/request/response/status;
   - storage/localStorage keys;
   - state defaults and transitions;
   - visible UI behavior;
   - edge cases and error states.
4. Mark every difference:
   - `must-fix`;
   - `acceptable-change`;
   - `intentional-redesign`;
   - `unknown`.
5. Record recommended tests before implementation.

## Default output location

Update:

```text
docs/compat/reader-dev-openreader-gap-analysis.md
```

For large modules, create a focused file under `docs/compat/`.
