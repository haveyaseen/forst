# Forst Error System

## Overview

This directory contains specifications and architecture notes for Forst’s error handling: **nominal** errors, **`ensure`-first** failure for **`Result`**, and lowering to Go / typed clients.

## Documents

### Normative language design

- **[02 — First-class errors (normative)](./02-first-class-errors-normative.md)** — **`ensure`-only** failure for **`Result`**, **`Err(Nominal)`** guards, **`or`** typing (**LUB**), **inference** of **`F`**, **implicit** **`error N`** from constructors, and **§7** **out-of-scope** mapping/wrapping. **Supersedes** [12 §5.7](../optionals/12-result-primitives-without-ok-err.md) **nominal `return` failure** for **`Result`** functions in favor of **`ensure … or`**. **§9** tracks **compiler checklist** (shipped vs remaining).

### Architecture and related RFCs

- **[00-error-system-architecture.md](./00-error-system-architecture.md)** — Architecture sketch: base **`Error`**, **`forst/errors`**, tags, TS **optional Effect compatibility**, roadmap (no Effect mandate in implementation phases).
- **[01 — Ensure-only failure propagation](./01-ensure-only-failure-returns.md)** — Propagate **`Result`** failures with **`ensure … is Ok() or …`** (not **`if` + failure `return`**); **`Ok`/`Err`** remain **guards**; pairs with **02** and **[12](../optionals/12-result-primitives-without-ok-err.md)**.

### Related language design (optionals hub)

- **[optionals hub](../optionals/00-crystal-inspired-optionals.md)** — Index of **`T | Nil`**, **`Result`**, and related topic docs (**01–09**).
- **[optionals / Result and error types](../optionals/02-result-and-error-types.md)** — How **`Result(Value, ErrorSubtype)`** ties **`Result`** to the **`Error`** hierarchy.
- **[optionals / Result primitives (RFC 12) §5.7](../optionals/12-result-primitives-without-ok-err.md)** — **`return x`** **success**; **failure** authoring for **`Result`** is refined by **02** ( **`ensure … or`** normative for failures).  
- **[optionals / Result primitives (RFC 12) §7](../optionals/12-result-primitives-without-ok-err.md#7-error-system-rfc--relationship-and-what-can-be-inferred)** — How the **errors** story informs **`Result`** **construction** and inference.

## Key Features

### 1. Structured Error Types

- **Hierarchical error system** with base and specific error types
- **Type-safe error creation** with factory functions
- **Rich error context** with tracing and metadata

### 2. Observability Integration

- **OpenTelemetry support** for distributed tracing
- **Structured logging** with trace IDs and context
- **Metrics collection** for error monitoring
- **Alerting integration** for critical errors

### 3. Effect Integration

- **Tagged error conversion** from Forst to Effect
- **Type-safe error handling** in TypeScript
- **Effect.gen patterns** for error composition
- **Resource management** for error handling

### 4. Modern Practices

- **Circuit breakers** for fault tolerance
- **Retry strategies** with exponential backoff
- **Error classification** (retryable vs non-retryable)
- **Graceful degradation** patterns

## Error Type Hierarchy

```
Error (Base)
├── ValidationError
├── DatabaseError
├── NetworkError
├── BusinessError
└── SystemError
```

## Implementation Phases

### Phase 1: Core Error System

- Base error types in Forst
- Error factory functions
- Context and tracing support

### Phase 2: Go Implementation

- Go error types and interfaces
- OpenTelemetry integration
- Error serialization

### Phase 3: TypeScript Integration

- Effect tagged errors
- Error conversion functions
- Effect error handling

### Phase 4: API and Observability

- HTTP error responses
- Status code mapping
- Observability tools integration

### Phase 5: Testing and Documentation

- Unit and integration tests
- Documentation and examples
- Tutorials and guides

### Phase 6: Production Readiness

- Performance optimization
- Security review
- Production monitoring

## Quick Start

See **[02 — First-class errors (normative)](./02-first-class-errors-normative.md)** for the full rules. Sketch:

### 1. Declare or infer a nominal error

Explicit:

```ft
error BadEmail {
    field: String
}
```

Or introduce **`BadEmail`** implicitly from the first **`ensure … or BadEmail{ … }`** (shape inferred).

### 2. Fail only via `ensure` on `Result`

```ft
func parsePositive(n: Int): Result(Int, NotPositive)
  ensure n > 0 or NotPositive{ field: "n" }
  return n
```

### 3. TypeScript clients

Generated types use a stable **`forstTag` / `_tag`**; optional compatibility with Effect-style tagged errors is described in [00 — Phase 3](./00-error-system-architecture.md#phase-3-typescript--client-integration-weeks-5-6). Effect is **not** required.

## Benefits

### For Developers

- **Type safety** with compile-time error checking
- **Clear error messages** with actionable information
- **Explicit failures** via **`ensure`** and nominal constructors
- **Rich debugging context** with trace IDs

### For Operations

- **Distributed tracing** across services
- **Error monitoring** and alerting
- **Performance tracking** and metrics
- **Fault tolerance** with circuit breakers

### For Production

- **Reliable error handling** with retry strategies
- **Observability** with OpenTelemetry
- **Monitoring** with structured logs and metrics
- **Alerting** for critical errors

## Best Practices

1. **Always include context** - Every error should have trace ID and relevant context
2. **Use specific error types** - Create specific types for different scenarios
3. **Include retry information** - Clearly indicate if an error is retryable
4. **Provide actionable messages** - Error messages should help developers understand what went wrong
5. **Maintain error hierarchy** - Use inheritance to create clear error relationships

## Integration

### With TypeScript runtimes

- Generated error shapes can align with **tagged** / **discriminated** unions; **Effect**-compatible patterns are **optional** ([00](./00-error-system-architecture.md)).

### With OpenTelemetry

- Automatic error recording in spans
- Error attributes and context
- Distributed tracing across services
- Metrics collection for error rates

### With Modern Tools

- Structured logging with ELK stack
- Metrics with Prometheus and Grafana
- Alerting with PagerDuty or similar
- Monitoring with DataDog or similar

This error system positions Forst as a production-ready platform with excellent error handling that integrates seamlessly with modern observability tools and Effect's error system.


