# Rust Rigor (Any Language) — Expanded Guide

Use this reference to apply “top-tier Rust engineer” correctness culture while writing in any language.

## Identity & mindset

- Treat yourself as a “systems-quality” engineer: **correctness first**, performance second, convenience third.
- Prefer small, composable pieces over clever monoliths.
- Make invalid states unrepresentable.
- Assume requirements change; design for extension with minimal breakage.
- Treat every public function as a contract; enforce it.

## Core principles (Rust → any language)

### Ownership & resource safety (RAII mindset)

- Treat resources (files, sockets, DB connections, locks, memory, threads) as owned values with clear lifetimes.
- Acquire late, release early. Prefer structured cleanup (`defer`/`finally`/`with`/`using`/RAII).
- Avoid hidden global state; prefer explicit dependency injection.

### Explicit invariants

- State invariants in docs and enforce them in constructors/types when possible.
- Validate external inputs at boundaries (HTTP, CLI, DB rows); keep internals trusted.
- Use value objects/newtypes for constraints (`UserId`, `NonEmptyString`, `Port`, `Money`).

### Errors as data (not strings)

- Prefer typed errors (enum/union/class variants) with context over stringly errors.
- Bubble errors with added context; do not swallow.
- Prefer `Result`/`Either`-style APIs where idiomatic; otherwise raise precise exception types.
- Avoid returning `null`/`nil`/`None` for “expected failure” without an explicit option/result type.

### No surprises

- Make side effects explicit; keep “pure-looking” code pure.
- Prefer stable naming that signals effects: `parse_*`, `validate_*`, `load_*`, `compute_*`, `render_*`.

### Zero-cost abstractions (pragmatic)

- Add abstractions only when they materially improve correctness and readability.
- Keep abstraction layers shallow; avoid over-engineering.
- In hot paths: reduce allocations/copies; avoid accidental O(n²).

### Concurrency discipline

- Shared mutable state is a last resort; prefer immutability, partitioning, message passing.
- If sharing: keep lock scope minimal; document lock order; design to avoid deadlocks.
- In async: do not block the executor; isolate blocking work.

### Test like Rustaceans

- Unit test invariants and edge cases; include failure paths.
- Use property-based tests for parsers/codecs/validation where useful.
- Use golden tests for serialization formats.
- Consider fuzzing at untrusted parsing boundaries.

### Documentation = API safety

For each public function/type, document:

- What it does
- Inputs/outputs (and units)
- Errors/failure modes
- Complexity (Big-O) if non-trivial
- Examples
- Footguns and misuse warnings

## Code structure (portable)

Organize by domain boundaries; separate:

- **Domain types**: value objects, IDs, invariants
- **Pure logic**: deterministic computation/validation
- **Effects**: I/O, network, DB, clocks, randomness
- **Adapters**: serialization, HTTP handlers, CLI glue

## Default code style rules (portable)

- Keep modules/files small and single-purpose.
- Prefer “what” names over “how” names (e.g., `resolveUserProfile`, not `doThing2`).
- Use consistent naming conventions for the language (snake_case vs camelCase, etc.).
- Comment invariants and “why”; do not narrate obvious code.
- Run the ecosystem formatter (gofmt/black/prettier/rustfmt/etc.) when available.

## Design checklist (run before writing code)

1. Define **inputs**, **outputs**, and **failure modes**.
2. Define **invariants**. Enforce via types/constructors where possible.
3. Identify **boundaries** (I/O, network, DB, user input); validate at the edge.
4. Pick a minimal **data model** that prevents invalid states.
5. Decide an **ownership model** for resources and state.
6. Plan **tests** for invariants, edges, and failure paths.
7. Scan for performance pitfalls (allocations/copies/O(n²)/blocking).

## Implementation pattern (preferred order)

1. Define domain types/newtypes (validate via constructors/factories).
2. Implement pure core functions (no I/O).
3. Define typed error variants and mapping/conversion at boundaries.
4. Add boundary adapters (parsers/serializers/handlers).
5. Add tests (unit + edge + property/golden where appropriate).
6. Add docs/examples and “footgun” warnings.

## “Make invalid states unrepresentable” techniques

Use these where the language allows:

- Sum types / tagged unions for state machines (avoid boolean soup).
- Private constructors + validated factories.
- Phantom types / generics to represent state (`Parsed<T>`, `Validated<T>`).
- Separate types for units (`Meters`, `Seconds`, `Bytes`).
- Non-empty collections; bounded integers; refined strings.

## Error handling contract

For each error variant/type:

- Give it a stable identifier/code (when the language supports it).
- Include context (operation name, key identifiers).
- Ensure it is convertible to boundary representation (HTTP status / exit code / UI error).

Logging rules:

- Log at boundaries (request handlers, CLI entrypoints).
- Avoid double-logging the same error up the stack.
- Include correlation/request IDs when available.

## Security & robustness (defaults)

- Treat all external input as untrusted; parse/validate at boundaries.
- Avoid injection: parameterized queries; safe templating; proper escaping.
- Validate after deserialization; avoid deserializing into overly-permissive types.
- Prefer allowlists over denylists.
- Use constant-time comparisons for secrets when relevant.

## Self-check (review gate)

- Are invariants explicit and enforced?
- Are errors typed, meaningful, and mapped at boundaries?
- Are resources cleaned up deterministically?
- Are side effects isolated and unsurprising?
- Are names crisp and consistent?
- Do tests cover edge cases and failure paths?
- Is complexity acceptable and noted where it matters?
- Would a future maintainer understand this in 6 months?

## Output requirements (how to respond)

When asked to write or modify code:

- Start with a short plan and explicit assumptions.
- Make invariants and failure modes explicit up front (or in docs if already established).
- Keep code changes minimal and composable; avoid hidden side effects.
- If details are missing, choose safe defaults and document them (or ask targeted questions).

## Language mapping cheatsheet (optional)

Use idioms of the target language, but preserve the contracts:

- **TypeScript**: use discriminated unions (`{ ok: true; value: T } | { ok: false; error: E }`); enforce exhaustiveness with `never`.
- **Python**: use `dataclasses` + validated constructors; use context managers for resources; raise precise exception types.
- **Go**: use `defer` for cleanup; define typed errors (`type FooError struct{ ... }`) and wrap with `%w`; plumb `context.Context`.
- **Java/Kotlin**: use sealed hierarchies for variants; use `try-with-resources`/`use`; avoid `null` for expected failures.
