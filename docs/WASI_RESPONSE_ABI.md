# WASI Invocation Response ABI

This document is the authoritative Alpha contract for data returned from a Strata add-on WASI invocation.

## Host-to-guest input

Invocation input is supplied on stdin using the existing versioned invocation envelope. Guest code does not control tenant scope, effective permission, database handles, credentials, host filesystem access, or raw networking.

## Guest-to-host response

For invocation operations, non-empty stdout MUST contain exactly one JSON object:

```json
{
  "schema_version": 1,
  "status_code": 200,
  "body": "SGVsbG8="
}
```

`body` is binary data represented by standard JSON base64 encoding. The host converts the validated envelope into `InvocationResult`.

Rules:

- `schema_version` MUST equal `1`.
- `status_code` MUST be between `200` and `599` inclusive.
- `body` is optional and remains bounded by the host runtime I/O limit.
- Unknown fields are rejected.
- Duplicate top-level fields are rejected; guests may not rely on last-value-wins JSON behavior.
- Malformed JSON, trailing JSON values, invalid base64, unsupported schema versions, invalid status codes, and oversized bodies fail closed.
- The host copies decoded body bytes across the trust boundary.
- Empty stdout retains the original Alpha compatibility behavior and maps to status `200` with an empty body.
- Health checks do not interpret stdout as an HTTP/module response.

The response ABI does not provide response headers, redirects, cookies, host capability selection, filesystem handles, credentials, raw sockets, database handles, NATS handles, or tenant scope.

## Engineering reinforcement

When changing this ABI or its decoder:

1. treat guest stdout as untrusted protocol input, not application-owned JSON;
2. use bounded reads before decoding;
3. reject unknown fields, duplicate fields, malformed/trailing values, unsupported versions, and out-of-range status values;
4. never expose decoder, runtime, filesystem, database, or broker error text directly to a guest-facing response;
5. test minimum/maximum sizes and malformed base64;
6. run the package tests and repository-wide race suite;
7. require Security Gate and exact-head CI before merge;
8. any head change invalidates earlier workflow evidence.

Do not expand this response envelope with host capabilities or sensitive metadata. New capabilities belong behind the reviewed capability broker and require their own authorization, tenancy, negative, and integration tests.
