---
name: api-contract-compat
description: API compatibility workflow for OpenReader. Use when changing routes, Gin handlers, auth, request bodies, query parameters, response schemas, status codes, API errors, or frontend API clients.
---

# API Contract Compatibility

Use this skill before modifying API behavior.

## Workflow

1. Extract the upstream reader-dev behavior when the feature exists upstream.
2. Compare current OpenReader route and JSON behavior.
3. Update `docs/compat/api-contract.md` and, when useful, `docs/compat/api-diff.md`.
4. Add or update a Go compatibility test or golden fixture.
5. Implement only after the contract is written.
6. Compare actual responses to the contract.

## Contract fields

Document:

- method and path;
- query parameters;
- request body;
- response body;
- status codes;
- auth requirement;
- side effects;
- error shape.

## Compatibility bias

Preserve stable OpenReader API paths for deployed clients, and preserve upstream semantics for feature behavior. If these conflict, document the shim or translation layer.
