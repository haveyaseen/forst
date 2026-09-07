# Forst Error System Architecture

## Table of Contents

1. [System Overview](#system-overview)
2. [Error types](#error-types-base-only-in-the-language)
3. [Error Context & Tracing](#error-context--tracing)
4. [Raising errors (`ensure`)](#raising-errors-ensure)
5. [Error Handling Patterns](#error-handling-patterns)
6. [Go Implementation](#go-implementation)
7. [TypeScript/Effect Integration](#typescripteffect-integration)
8. [API Design](#api-design)
9. [Observability Integration](#observability-integration)
10. [Testing Strategy](#testing-strategy)
11. [Best Practices](#best-practices)
12. [Implementation Roadmap](#implementation-roadmap)

---

## System Overview

### Core Principles

| Principle                | Description                            | Implementation                            |
| ------------------------ | -------------------------------------- | ----------------------------------------- |
| **Structured Errors**    | Every error has clear type and context | Base **`Error`** in language; nominals are user-defined |
| **Observability First**  | Built-in tracing and logging           | OpenTelemetry integration, trace IDs      |
| **Type Safety**          | Compile-time and runtime validation    | Forst types → Go types → TypeScript types |
| **Developer Experience** | Structured types and clear failure codes | Rich error context, Effect integration      |

### Error Flow Architecture

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Forst Code    │───▶│   Go Runtime     │───▶│  TypeScript     │
│                 │    │                  │    │                 │
│ • Error Types   │    │ • `forst/errors` │    │ • Effect Errors │
│ • Context       │    │ • Tracing        │    │ • Tagged Errors │
│ • `ensure`      │    │ • Serialization  │    │ • Error Handling│
└─────────────────┘    └──────────────────┘    └─────────────────┘
         │                       │                       │
         ▼                       ▼                       ▼
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Observability │    │   Error API      │    │   Error Client  │
│                 │    │                  │    │                 │
│ • OpenTelemetry │    │ • HTTP Responses │    │ • Error Mapping │
│ • Metrics       │    │ • Error Codes    │    │ • Retry Logic   │
│ • Logging       │    │ • Status Codes   │    │ • Circuit Breaker│
└─────────────────┘    └──────────────────┘    └─────────────────┘
```

---

## Error types (base only in the language)

The language provides a single abstract **base `Error`** for typing and lowering. **Authors never write `error Error { … }`** in source—that form does not exist in normative Forst. Concrete kinds (validation, I/O, domain, …) are **user nominals** declared with **`error X { … }`** only—there is **no** inheritance between user nominals (see [02 — First-class errors (normative)](./02-first-class-errors-normative.md)). You may declare nominals explicitly, or—**unless already defined**—introduce them **implicitly** the first time you use a constructor on the right-hand side of **`ensure`**. There are **no** built-in standard error variants such as `ValidationError` or `NetworkError`.

### 1. Base `Error` (language-defined, not a user `error` declaration)

The language does **not** require a human-readable string on every error. Optional display text may live on **specific** subtypes or **host** layers (HTTP bodies, logs)—not as a mandatory field on the base type.

Nominal **`error`** names in Forst correspond to **tags** in lowered Go (see **`forst/errors`** below); do not rely on `Error{}` strings for classification at boundaries.

The **implementation** supplies a base **`Error`** with fields along these lines (exact layout is specified with the compiler; this is **not** spelled as **`error Error { … }`** in Forst source):

- **Core:** `code`, `timestamp`
- **Tracing (optional):** `traceId`, `spanId`
- **Context (optional):** `context` as key–value metadata

### 2. User-defined nominal errors (explicit or implicit)

**Explicit:** declare the nominal and its payload shape in source (**inherits** the base **`Error`** automatically):

```ft
error NotPositive {
    field: String
}
```

**Implicit:** if **`NotPositive`** is not yet declared, a line such as **`ensure n > 0 or NotPositive{ field: "n" }`** causes the compiler to introduce **`error NotPositive { field: String }`** (the shape is inferred from the **constructor**—field names and types must be consistent across the package). Further **`ensure`** sites that use **`NotPositive{ … }`** must match that shape.

Other domains can use the same pattern (`RateLimited{ … }`, …). **Retryability**, **HTTP status**, and **alerting** are **application policy** over those nominals, not fixed tables in the language.

The classification examples later use a second illustrative nominal:

```ft
error IoTimeout {
    op: String
}
```

---

## Error Context & Tracing

### 1. Error Context Structure

```ft
struct ErrorContext {
    // Tracing identifiers
    traceId: String
    spanId: String

    // Request context
    userId: String?
    requestId: String?

    // Service context
    service: String
    version: String
    environment: String

    // Additional metadata
    metadata: Map[String, String]
}
```

### 2. Tracing and context at runtime

Trace IDs, span IDs, and service metadata may be attached when errors are **lowered** to Go or when crossing **host** boundaries—rather than via ad hoc “builder” or factory helpers in Forst source. The authoring model is **`ensure`** + nominal constructors (see below).

### 3. Tracing integration (reference)

| Component    | Purpose               | Implementation                  |
| ------------ | --------------------- | ------------------------------- |
| **Trace ID** | Request correlation   | Generated per request           |
| **Span ID**  | Operation correlation | Generated per operation         |
| **Context**  | Rich debugging info   | Service, user, request metadata |
| **Metadata** | Custom attributes     | Key-value pairs for debugging   |

---

## Raising errors (`ensure`)

In Forst, **domain failures are not produced by factory helpers or ad hoc “return an error value”** in the primary authoring model. Instead:

1. **`ensure`** — Failure is raised via **`ensure <condition> or <failure>`**. Only **`ensure`** introduces a failure along that path (see the [errors RFC hub](./README.md) and [ensure-only propagation](./01-ensure-only-failure-returns.md)).
2. **Nominal constructors** — The **`or`** branch uses a Go-style struct literal: empty **`RateLimited{}`** or keyed fields **`NotPositive{ field: "n" }`**. If the nominal is not yet declared, the compiler **implicitly defines** the corresponding **`error NotPositive { … }`** from that constructor (see [Error types — user-defined nominal errors](#2-user-defined-nominal-errors-explicit-or-implicit)).
3. **Forwarding** — When propagating a failure from a **`Result`** or inner call, use **`ensure`** with **`is Ok()`** (or the agreed **`Err`** guard) and **`or`** the constructor you need, or forward the inner failure expression when it already matches the expected nominal type—rather than building errors through generic factory functions.

### Positive framing (by design)

Requiring **`ensure`** steers authors toward **what must hold** for the function to continue—the **expected** precondition or outcome—rather than centering control flow on **unexpected** failure branches first. The condition reads as an **invariant** or **success criterion**; the **`or`** branch names the nominal failure only when that criterion is not met. This does not remove the need to think about failures, but it keeps the **happy path** and **explicit expectations** in the foreground.

Illustrative (syntax follows the normative errors / `ensure` RFCs):

```ft
func parsePositive(n: Int): Result(Int, NotPositive)
  ensure n > 0 or NotPositive{
    field: "n",
  }
  return n
```

After this line (if **`NotPositive`** was not declared earlier), the compiler treats **`NotPositive`** as **`error NotPositive { field: String }`** for the rest of the package.

This document does **not** prescribe **factory functions** (`newXError`, `NewXError`, etc.) for creating error values in Forst or in generated Go **call sites**—those patterns belong to legacy sketches; the **constructor + `ensure`** model replaces them for user-facing code.

---

## Error Handling Patterns

### 1. Error Processing Pipeline

```ft
func handleError(error: Error) Error {
    // 1. Log error with context
    logError(error)

    // 2. Record metrics
    recordErrorMetrics(error)

    // 3. Check retry eligibility
    if isRetryableError(error) {
        scheduleRetry(error)
    }

    // 4. Check alert requirements
    if shouldAlert(error) {
        sendAlert(error)
    }

    return error
}
```

### 2. Error classification (policy over *your* nominals)

Retry and alerting are **not** fixed by the language. You match on **nominal types you declared** (here `NotPositive` is the illustrative error from §2 above; `IoTimeout` stands in for another error you might define elsewhere).

```ft
func isRetryableError(error: Error) Bool {
    switch error {
    case NotPositive:
        return false
    case IoTimeout:
        return true
    default:
        return false
    }
}

func shouldAlert(error: Error) Bool {
    switch error {
    case IoTimeout:
        return true
    case NotPositive:
        return false
    default:
        return false
    }
}
```

### 3. Error Handling Strategies

| Strategy            | When to Use           | Implementation               |
| ------------------- | --------------------- | ---------------------------- |
| **Fail Fast**       | Precondition failures | Return immediately           |
| **Retry**           | Transient errors      | Exponential backoff          |
| **Circuit Breaker** | Service failures      | Stop calling failing service |
| **Fallback**        | Non-critical failures | Use default values           |
| **Alert**           | Critical errors       | Send notifications           |

---

## Go Implementation

### 1. Tags, `forst/errors`, and coexistence with Go errors

Forst errors should be identifiable at runtime **without** guessing from `err.Error()` strings (which collide easily with **`fmt.Errorf`**, libraries, and opaque wrappers). The intended design:

1. **Stable tags** — Each nominal Forst error maps to a Go value that carries a **distinct tag** (e.g. a `Tag` field or a `ForstTag() string` method), aligned with the **nominal name** in Forst and with TypeScript **tagged** / **discriminated** shapes on the client. Tags are used for **`errors.As`**, logging filters, and HTTP/JSON mapping—not for replacing `Error() string`.

2. **Standard package `forst/errors`** — The compiler **depends on** and **generates against** a small, versioned Go module (import path conceptually **`forst/errors`**, analogous to built-in stdlib: always available, not something every app vendors by hand). It should define:
   - **Shared interfaces** (e.g. a **`ForstError`** marker that includes **`Tag()`** or **`ForstTag()`**),
   - **Optional helpers** such as **`IsForst(err error) bool`**, **`TagOf(err error) (string, bool)`**, or unwrap-safe predicates,
   - **Base struct fields** shared by lowered errors (codes, trace IDs) so user packages and generated code stay consistent.

3. **Minimal conflict with typical `error` values** — Plain Go errors remain plain: **`forst/errors`** types should **not** overload **`Error{}`** to embed the only copy of structured data; use normal struct fields and **`Unwrap`** where wrapping. Third-party **`error`** values are not Forst errors unless **`errors.As`** into a **user-generated** concrete type (e.g. `*NotPositiveError`) or the tag helper succeeds. Namespace **JSON** field names if needed to avoid clashing with generic API envelopes.

Generated concrete types (e.g. `*NotPositiveError`) **live in the user’s Go module** but **embed** or **satisfy** types from **`forst/errors`**; the **tag** and **shape** follow the user’s Forst declarations.

### 2. Error interface

```go
// Base error interface (implemented by generated types; definitions anchored in forst/errors)
type ForstError interface {
    error
    ForstTag() string // stable nominal tag; never inferred from Error() string alone
    Code() string
    Timestamp() int64
    TraceID() string
    SpanID() string
    Context() map[string]string
    ToJSON() string
}

// Base error implementation (no required human-readable message field).
// Tag identifies the user-defined nominal kind (e.g. "NotPositive"); see §1 above.
type BaseError struct {
    Tag       string            `json:"forstTag"` // stable discriminator; not parsed from Error() string
    Code      string            `json:"code"`
    Timestamp int64             `json:"timestamp"`
    TraceID   string            `json:"traceId"`
    SpanID    string            `json:"spanId"`
    Context   map[string]string `json:"context,omitempty"`
}

func (e *BaseError) Error() string {
    return e.Code // human-facing summary; Tag/fields are authoritative for classification
}

func (e *BaseError) ForstTag() string {
    return e.Tag
}

func (e *BaseError) Code() string {
    return e.Code
}

func (e *BaseError) Timestamp() int64 {
    return e.Timestamp
}

func (e *BaseError) TraceID() string {
    return e.TraceID
}

func (e *BaseError) SpanID() string {
    return e.SpanID
}

func (e *BaseError) Context() map[string]string {
    return e.Context
}

func (e *BaseError) ToJSON() string {
    data, _ := json.Marshal(e)
    return string(data)
}
```

### 3. Generated concrete type (illustrative)

The language does not ship `ValidationError`, `DatabaseError`, etc. Each nominal you declare becomes its own generated struct. One example matching the **`NotPositive`** sketch:

```go
// Generated in your module from `error NotPositive { field: String }` — not a built-in type.
type NotPositiveError struct {
    BaseError
    Field string `json:"field"`
}

func (e *NotPositiveError) Error() string {
    return fmt.Sprintf("not positive: %s", e.Field)
}
```

Go **call sites** in Forst do not hand-author these structs to **fail**; the compiler lowers **`ensure … or NotPositive{ … }`** into values like `*NotPositiveError`. Other nominals get analogous generated types with **distinct `ForstTag()`** values.

### 4. OpenTelemetry integration

```go
// OpenTelemetry integration
type ErrorTracer struct {
    tracer trace.Tracer
}

func NewErrorTracer() *ErrorTracer {
    return &ErrorTracer{
        tracer: otel.Tracer("forst-errors"),
    }
}

func (et *ErrorTracer) RecordError(ctx context.Context, err ForstError) {
    span := trace.SpanFromContext(ctx)
    if span.IsRecording() {
        span.RecordError(err)
        span.SetStatus(codes.Error, err.Error())

        // Add error attributes
        span.SetAttributes(
            attribute.String("error.forst_tag", err.ForstTag()),
            attribute.String("error.code", err.Code()),
            attribute.String("error.trace_id", err.TraceID()),
            attribute.String("error.span_id", err.SpanID()),
        )

        // Add context attributes
        for key, value := range err.Context() {
            span.SetAttributes(attribute.String("error.context."+key, value))
        }
    }
}

func (et *ErrorTracer) RecordErrorWithSpan(ctx context.Context, err ForstError, spanName string) {
    ctx, span := et.tracer.Start(ctx, spanName)
    defer span.End()

    et.RecordError(ctx, err)
}
```

---

## TypeScript/Effect Integration

### 1. Effect tagged errors (base + user nominals)

Only the **base** tagged class mirrors the built-in **`Error`**; every other kind is **emitted from your Forst declarations** (here `NotPositive` and `IoTimeout` match the illustrations above).

```typescript
// Base — corresponds to the language abstract Error (no `error Error {}` in source)
export class ForstError extends Data.TaggedError("ForstError")<{
  code: string;
  timestamp: number;
  traceId: string;
  spanId: string;
  context?: Record<string, string>;
}> {}

// Generated per user nominal — not built into the language
export class NotPositiveError extends Data.TaggedError("NotPositive")<{
  code: string;
  timestamp: number;
  traceId: string;
  spanId: string;
  context?: Record<string, string>;
  field: string;
}> {}

export class IoTimeoutError extends Data.TaggedError("IoTimeout")<{
  code: string;
  timestamp: number;
  traceId: string;
  spanId: string;
  context?: Record<string, string>;
  op: string;
}> {}
```

### 2. Error conversion functions (wire / JSON boundaries)

These reconstruct tagged errors from **transport** payloads (HTTP, RPC). They are **not** the Forst authoring model, which uses **`ensure`** and nominal constructors in source.

```typescript
// Wire conversion: branch on `forstTag` / `_tag` from your API contract — not on fixed built-in kinds.
export function convertForstError(error: any): ForstError {
  if (error.forstTag === "NotPositive" || error._tag === "NotPositive") {
    return new NotPositiveError{
      code: error.code,
      timestamp: error.timestamp,
      traceId: error.traceId,
      spanId: error.spanId,
      context: error.context,
      field: error.field,
    };
  }
  if (error.forstTag === "IoTimeout" || error._tag === "IoTimeout") {
    return new IoTimeoutError{
      code: error.code,
      timestamp: error.timestamp,
      traceId: error.traceId,
      spanId: error.spanId,
      context: error.context,
      op: error.op,
    };
  }
  return new ForstError{
    code: error.code || "UNKNOWN_ERROR",
    timestamp: error.timestamp || Date.now(),
    traceId: error.traceId || "",
    spanId: error.spanId || "",
    context: error.context,
  };
}
```

### 3. Effect Error Handling

```typescript
// Effect error handling
export class ForstErrorHandler {
  static handleError = (
    error: ForstError
  ): Effect.Effect<never, ForstError, never> =>
    Effect.gen(function* () {
      // Log error
      yield* Effect.logError("Forst error occurred", error);

      // Record metrics
      yield* Effect.sync(() => {
        console.log(`Error recorded: ${error.code}`);
      });

      // Check if error should be retried
      if (ForstErrorHandler.isRetryableError(error)) {
        yield* Effect.log("Error is retryable, scheduling retry");
      }

      // Check if error should trigger alert
      if (ForstErrorHandler.shouldAlert(error)) {
        yield* Effect.log("Error should trigger alert");
      }

      return error;
    });

  static isRetryableError = (error: ForstError): boolean => {
    if (error instanceof IoTimeoutError) return true;
    if (error instanceof NotPositiveError) return false;
    return false;
  };

  static shouldAlert = (error: ForstError): boolean => {
    if (error instanceof IoTimeoutError) return true;
    return false;
  };
}
```

### 4. Effect Services with Error Handling

```typescript
// Effect services with error handling
export class ForstService {
  static processUser = (id: number) =>
    Effect.gen(function* () {
      // Call Forst function
      const result = yield* Effect.tryPromise{
        try: () => forstClient.processUser(id),
        catch: (error) => convertForstError(error),
      };

      // Handle different error types
      if (result.success) {
        return result.data;
      } else {
        const forstError = convertForstError(result.error);
        yield* ForstErrorHandler.handleError(forstError);
        return yield* Effect.fail(forstError);
      }
    });

  static processUserWithRetry = (id: number) =>
    Effect.retry(ForstService.processUser(id), {
      times: 3,
      delay: "exponential",
    });

  static processUserWithTimeout = (id: number) =>
    Effect.timeout(ForstService.processUser(id), "5 seconds");

  static processUserWithCircuitBreaker = (id: number) =>
    Effect.gen(function* () {
      // Circuit breaker logic
      const result = yield* Effect.tryPromise{
        try: () => circuitBreaker.call(() => forstClient.processUser(id)),
        catch: (error) => convertForstError(error),
      };

      if (result.success) {
        return result.data;
      } else {
        const forstError = convertForstError(result.error);
        yield* ForstErrorHandler.handleError(forstError);
        return yield* Effect.fail(forstError);
      }
    });
}
```

---

## API Design

### 1. Error Response Format

```typescript
// Standardized error response format — payload shape depends on the nominal `forstTag`
export interface ForstErrorResponse {
  success: false;
  error: {
    type: string;
    forstTag: string;
    /** Optional human-readable text; not required by the Forst error model. */
    message?: string;
    code: string;
    timestamp: number;
    traceId: string;
    spanId: string;
    context?: Record<string, string>;
    [key: string]: unknown;
  };
}
```

### 2. HTTP status mapping

There is **no** fixed global table: HTTP codes are **application policy** keyed by **`forstTag`** (and optionally `code`). Example: map `NotPositive` → 400, `IoTimeout` → 504; your service defines the rest for its own nominals.

### 3. Error Handling Middleware

```typescript
// Error handling middleware
export const errorHandlingMiddleware = (
  req: Request,
  res: Response,
  next: NextFunction
) => {
  try {
    // Process request
    next();
  } catch (error) {
    const forstError = convertForstError(error);

    // Log error
    console.error("Request error:", forstError);

    // Record metrics
    recordErrorMetrics(forstError);

    // Send error response
    res.status(getHttpStatusFromError(forstError)).json({
      success: false,
      error: {
        type: forstError._tag,
        forstTag: forstError._tag,
        code: forstError.code,
        timestamp: forstError.timestamp,
        traceId: forstError.traceId,
        spanId: forstError.spanId,
        context: forstError.context,
        ...getTypeSpecificFields(forstError),
      },
    });
  }
};

function getHttpStatusFromError(error: ForstError): number {
  if (error instanceof NotPositiveError) return 400;
  if (error instanceof IoTimeoutError) return 504;
  return 500;
}

function getTypeSpecificFields(error: ForstError): Record<string, unknown> {
  if (error instanceof NotPositiveError) {
    return { field: error.field, constraint: error.constraint };
  }
  if (error instanceof IoTimeoutError) {
    return { op: error.op };
  }
  return {};
}
```

---

## Observability Integration

### 1. OpenTelemetry Integration

```typescript
// OpenTelemetry integration
import { trace, context } from "@opentelemetry/api";

export class OpenTelemetryErrorHandler {
  static recordError = (error: ForstError): Effect.Effect<void, never, never> =>
    Effect.sync(() => {
      const span = trace.getActiveSpan();
      if (span) {
        span.recordException(error);
        span.setStatus{ code: 1, message: error.code };

        // Add error attributes
        span.setAttributes{
          "error.code": error.code,
          "error.trace_id": error.traceId,
          "error.span_id": error.spanId,
          "error.type": error._tag,
        };

        // Add context attributes
        if (error.context) {
          Object.entries(error.context).forEach(([key, value]) => {
            span.setAttributes{ [`error.context.${key}`]: value };
          });
        }
      }
    });

  static recordErrorWithSpan = (error: ForstError, spanName: string) =>
    Effect.gen(function* () {
      const tracer = trace.getTracer("forst-errors");
      const span = tracer.startSpan(spanName);

      yield* Effect.sync(() => {
        span.recordException(error);
        span.setStatus{ code: 1, message: error.code };
        span.end();
      });
    });
}
```

### 2. Metrics Integration

```typescript
// Metrics integration
export class ErrorMetrics {
  static recordError = (error: ForstError): Effect.Effect<void, never, never> =>
    Effect.sync(() => {
      // Record error counter
      console.log(`Error counter: ${error.code}`);

      // Record error rate
      console.log(`Error rate: ${error.code}`);

      // Record error duration
      console.log(`Error duration: ${error.code}`);
    });

  static recordErrorRate = (
    error: ForstError
  ): Effect.Effect<void, never, never> =>
    Effect.sync(() => {
      // Record error rate metrics
      console.log(`Error rate recorded: ${error.code}`);
    });
}
```

---

## Testing Strategy

### 1. Unit Testing

```typescript
// Error testing
import { describe, it, expect } from "vitest";
import { Effect } from "effect";

describe("Forst Error Handling", () => {
  it("should convert NotPositive wire payload", () => {
    const error = {
      forstTag: "NotPositive",
      code: "NOT_POSITIVE",
      field: "n",
    };

    const forstError = convertForstError(error);

    expect(forstError).toBeInstanceOf(NotPositiveError);
    expect(forstError.field).toBe("n");
  });

  it("should treat IoTimeout as retryable", () => {
    const error = new IoTimeoutError{
      code: "IO_TIMEOUT",
      timestamp: Date.now(),
      traceId: "trace-123",
      spanId: "span-456",
      op: "read",
    };

    expect(ForstErrorHandler.isRetryableError(error)).toBe(true);
  });

  it("should treat NotPositive as non-retryable", () => {
    const error = new NotPositiveError{
      code: "NOT_POSITIVE",
      timestamp: Date.now(),
      traceId: "trace-123",
      spanId: "span-456",
      field: "n",
    };

    expect(ForstErrorHandler.isRetryableError(error)).toBe(false);
  });
});
```

### 2. Integration Testing

```typescript
// Integration testing
describe("Forst Service Integration", () => {
  it("should handle Forst errors correctly", async () => {
    const program = Effect.gen(function* () {
      const result = yield* ForstService.processUser(123);
      return result;
    });

    const result = await Effect.runPromise(program);

    if (result._tag === "Left") {
      expect(result.left).toBeInstanceOf(ForstError);
    } else {
      expect(result.right).toBeDefined();
    }
  });
});
```

---

## Best Practices

### 1. Error Design Principles

| Principle                       | Description                                                             | Implementation                           |
| ------------------------------- | ----------------------------------------------------------------------- | ---------------------------------------- |
| **Always include context**      | Trace and metadata when lowering or at boundaries                      | Compiler / runtime + host layers         |
| **Use specific error types**    | Don't use generic errors, create specific types for different scenarios | Type hierarchy with specific error types |
| **Include retry information**   | Clearly indicate if an error is retryable                               | `retryable` field in error types         |
| **Optional human-readable text**| Add strings at HTTP/log boundaries when helpful; not a language requirement | Subtype fields or host layers              |
| **Inherit base `Error`**        | User nominals inherit the language base automatically                   | `error MyKind { … }` (no `extends Error`)  |

### 2. Error Handling Patterns

| Pattern                  | When to Use                                           | Implementation                             |
| ------------------------ | ----------------------------------------------------- | ------------------------------------------ |
| **Fail fast**            | Don't continue processing after encountering an error | Return immediately on precondition failures |
| **Log errors**           | Always log errors with full context                   | Structured logging with trace IDs          |
| **Record metrics**       | Track error rates and types                           | Metrics collection for monitoring          |
| **Use circuit breakers** | Prevent cascading failures                            | Circuit breaker pattern for external calls |
| **Implement retries**    | Handle transient errors gracefully                    | Exponential backoff for retryable errors   |

### 3. Observability Best Practices

| Practice                          | Description                                    | Implementation                        |
| --------------------------------- | ---------------------------------------------- | ------------------------------------- |
| **Use structured logging**        | Include trace IDs and context in all logs      | JSON logging with structured fields   |
| **Record error metrics**          | Track error rates, types, and patterns         | Prometheus metrics for error tracking |
| **Implement distributed tracing** | Follow requests across service boundaries      | OpenTelemetry integration             |
| **Set up alerting**               | Alert on critical errors and error rate spikes | Alerting rules for error thresholds   |
| **Monitor error trends**          | Track error patterns over time                 | Dashboards for error trend analysis   |

---

## Implementation Roadmap

### Phase 1: Core Error System (Weeks 1-2)

- [ ] Implement base `Error` type in Forst (no other built-in error kinds)
- [ ] Lower `ensure … or Constructor(...)` to idiomatic Go `error` values
- [ ] Implement error context and tracing
- [ ] Add basic error handling patterns

### Phase 2: Go Implementation (Weeks 3-4)

- [ ] Generate user nominal error structs in Go; ship `forst/errors` base only
- [ ] Add OpenTelemetry integration
- [ ] Create error serialization
- [ ] Implement error handling middleware

### Phase 3: TypeScript / client integration (Weeks 5-6)

- [ ] Emit tagged or discriminated TypeScript types for user nominal errors (aligned with `forstTag` / `_tag`; see **Effect compatibility** below)
- [ ] Implement wire / JSON conversion into those types at API boundaries
- [ ] Document client-side handling patterns (narrowing, retries) without mandating a specific runtime library
- [ ] Ensure generated shapes are **compatible** with [Effect](https://github.com/Effect-TS/effect)’s error model where practical (teams may wrap values with `Data.TaggedError` or unions with `Data.taggedEnum` themselves—Forst does not depend on Effect)

#### TypeScript: optional compatibility with Effect (analysis)

Effect documents **tagged errors** and **native `cause`** under [Data — TaggedError](https://effect.website/docs/data-types/data/#taggederror) and [Native cause support](https://effect.website/docs/data-types/data/#native-cause-support). The implementation lives primarily in [`packages/effect/src/Data.ts`](https://github.com/Effect-TS/effect/blob/main/packages/effect/src/Data.ts).

**What Effect assumes (relevant to Forst-generated types):**

1. **`_tag` discriminant** — `Data.tagged` / `Data.TaggedClass` / `TaggedEnum` all use a readonly **`_tag`** field whose value is the variant name (`Data.ts`: `tagged` sets `value._tag = tag`; `TaggedClass` sets `readonly _tag = tag`). **`Data.TaggedError(tag)`** builds a class whose instances extend `Data.Error` and set **`readonly _tag = tag`** (see `TaggedError` at the bottom of `Data.ts`). This matches Forst’s story of **nominal names → stable string tags**; generated TS should expose the same discriminator so `Predicate.isTagged`, **`Effect.catchTag`**, and manual `switch (e._tag)` all work if consumers opt in.

2. **`Data.Error` and `cause`** — `Data.Error`’s constructor calls `super(args?.message, args?.cause ? { cause: args.cause } : undefined)` and **`Object.assign(this, args)`**, so arbitrary fields (including **`cause`**) sit on the error object and integrate with **JavaScript’s `Error.prototype.cause`**. `TaggedError` subclasses **`Data.Error`**, so instances can carry **`cause`** the same way. Forst-generated TS types do **not** need to subclass Effect’s classes; they only need to remain **structurally usable** where teams pass payloads into **`Data.TaggedError("MyTag")<Payload>()`** or mirror `{ readonly _tag: "MyTag"; … }` unions.

3. **No mandate** — Compatibility means: **discriminated unions / `_tag`**, optional alignment of field names (`message`, `cause`) with `Data.Error` conventions, and documentation. It does **not** mean generating **`import { Data } from "effect"`** in user code by default.

### Phase 4: API and Observability (Weeks 7-8)

- [ ] Implement error response format
- [ ] Add HTTP status code mapping
- [ ] Create error handling middleware
- [ ] Integrate with observability tools

### Phase 5: Testing and Documentation (Weeks 9-10)

- [ ] Add comprehensive unit tests
- [ ] Create integration tests
- [ ] Write documentation
- [ ] Add examples and tutorials

### Phase 6: Production Readiness (Weeks 11-12)

- [ ] Performance optimization
- [ ] Security review
- [ ] Production testing
- [ ] Monitoring and alerting setup

---

## Conclusion

This comprehensive error handling system provides:

### **Key Benefits**

1. **Type Safety** - Compile-time and runtime error type checking
2. **Observability** - Built-in tracing, logging, and metrics
3. **Developer Experience** - Structured types and clear failure codes; optional human-readable text at boundaries
4. **Production Ready** - Circuit breakers, retries, and alerting
5. **Client types** - Generated or hand-maintained TS shapes that mirror nominal errors for boundaries

### **Modern Practices**

1. **Structured Errors** - Base `Error` plus nominals you define
2. **Distributed Tracing** - OpenTelemetry integration
3. **Error Classification** - Retryable vs non-retryable errors
4. **Circuit Breakers** - Fault tolerance patterns
5. **Metrics and Alerting** - Production monitoring

This approach positions Forst as a production-ready system with excellent error handling that integrates cleanly with observability tools and typed clients at HTTP and RPC boundaries.


