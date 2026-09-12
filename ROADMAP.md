# Forst Roadmap

Forst aims to give backend developers TypeScript-grade ergonomics while compiling to Go and interoperating with TypeScript clients ([README](./README.md)). Principles and boundaries live in [PHILOSOPHY.md](./PHILOSOPHY.md).

## How this roadmap works

This document tracks **what exists**, **what is in flight**, and **what we refuse to add**—aligned with [PHILOSOPHY.md](./PHILOSOPHY.md)—without shipping dates (use issues/milestones for scheduling).

Each section below is a **feature parity** table: **Feature** | **Status** | **Notes**. Status values:

- ✅ **done** — Believed complete for current scope; report gaps as bugs.
- ⏳ **in progress** — Actively being implemented **toward** the **done** bar; remaining gaps are temporary.
- 🔬 **experimental** — Something **exists** (often prototype), but scope, stability, or polish are **not** at the **done** bar yet—may be a thin surface, partial behavior, or “works, but don’t rely on the contract long-term” while the surface is still maturing.
- 📋 **planned** — On the roadmap but **not yet delivered**: no implementation yet.
- ❓ **unclear** — The use cases or design implications are not understood well enough to decide whether the feature belongs on the roadmap.
- 🚫 **not planned (anti-feature)** — Intentionally **omitted** from the language design; not a backlog gap—see [Anti-features](#anti-features) and [PHILOSOPHY.md](./PHILOSOPHY.md).

**In progress vs experimental:** **In progress** means implementation is **underway** toward **done**. **Experimental** means the feature is **out in the wild** in some form, but we are **not** yet treating it as complete—whether because large pieces are missing, behavior may change, or advertised capabilities are still stubs.

Themes group work (language, interop, tooling, docs, infrastructure). We do not publish dates here; use GitHub issues and milestones for concrete execution.

---

## Anti-features

These items are **not** on the roadmap as future work—they are **deliberately excluded** where they would fight predictability, explicit control flow, or Go-aligned error handling. They implement the “**no unpredictable behavior**” and related boundaries in [PHILOSOPHY](./PHILOSOPHY.md#anti-features): features that hide control flow, surprise with errors, or break compile-time reasoning stay out of the language surface.

| Topic | 🚫 Why not planned |
| --- | --- |
| **`panic` / `recover` as language constructs** | Non-local, implicit control flow; hard to trace and compose. Forst steers toward explicit **`Result`** / **`error`** paths instead—see [Optional & Result Types](#optional--result-types) and [Result & error types](./examples/in/rfc/optionals/02-result-and-error-types.md). (Interop may still surface `panic` in generated Go.) |
| **Exceptions (`try` / `catch` / `throw`)** | Same family as above: surprise jumps up the stack. Aligns with Go’s **`error`** returns and **`Result(S, F)`** rather than TS/Java-style exceptions. |
| **Class hierarchies and deep inheritance trees** | Inheritance obscures which fields an API actually has; Forst favors **structural shapes** and **function-centric** APIs—see [No needless object orientation](./PHILOSOPHY.md#no-needless-object-orientation). |
| **Macros, preprocessors, compile-time code that rewrites control flow** | Easy to hide behavior; PHILOSOPHY requires control-flow changes to use **ordinary keywords** so execution stays readable. |
| **Runtime reflection for wiring and validation** | Providers, constraints, and obligation tracking are **compile-time**; Forst does not discover or wire services via introspection at runtime. |
| **TypeScript-style `undefined` as a separate value** | Dual null/undefined semantics complicate APIs and generated types; the optionals direction is **`Nil`** / absence without `undefined`—see [optionals RFC hub](./examples/in/rfc/optionals/README.md). |
| **Implicit numeric / widening coercion** | Silent `int`↔`float` (and similar) breaks predictability for backends; conversions should be **explicit**—see [PHILOSOPHY](./PHILOSOPHY.md#no-implicit-type-conversions). |
| **Dependent types & arbitrary type-level computation** | Keeps typechecking decidable and tooling fast; no type families / template metaprogramming—see [No side effects in type system](./PHILOSOPHY.md#no-side-effects-in-type-system) (and nested headings there). |
| **Runtime type mutation (“monkey patching”)** | Types are fixed at compile time so behavior stays auditable—see [PHILOSOPHY](./PHILOSOPHY.md#no-runtime-type-modifications). |

---

## Language & types

The language surface is organized around **structural types**, **explicit annotations where inference would be ambiguous**, and **Go-shaped** execution—see [Guiding Principles](./PHILOSOPHY.md#guiding-principles). Subsections below group roadmap items by concern (core types, optional/result modeling, guards, generics, control flow, builtins).

### Core typing & declarations

**Intention:** Establish the baseline **static type system**, modules, and declarations so APIs are checkable and stable—aligned with “robust type checking” and “predictable, deterministic behavior” in [PHILOSOPHY](./PHILOSOPHY.md#guiding-principles).

| Feature | Status | Notes |
| --- | --- | --- |
| Basic type system | ✅ done | Core static typing. |
| Shape-based types | ✅ done | Structural shapes. |
| Type definitions | ✅ done | User-defined types. |
| Packages, imports, top-level `func` | ✅ done | Core compilation path (same role as Go). |
| `var`, assignments, `:=`, short declarations | ✅ done | Forst uses typed `name: Type =` and inference-friendly forms alongside Go-like patterns. Compound assignment (`+=`, `-=`, `*=`, `/=`, `%=`, `&=`, `\|=`) and postfix `++` / `--` are supported—see control-flow rows below. |

### Optional & Result Types

**Intention:** Model **absence** and **failure** in the type system instead of ad hoc conventions: optional values without a separate **`undefined`** universe (see [optionals direction](./examples/in/rfc/optionals/README.md)), and **`Result`**-style success/failure that still maps to idiomatic Go **`(T, error)`**. This supports [traceable errors](./PHILOSOPHY.md#no-surprising-errors) and explicit handling rather than exceptions or non-local jumps ([Anti-features](#anti-features)); structured error tagging builds on the same story ([errors RFC](./examples/in/rfc/errors/README.md)).

| Feature | Status | Notes |
| --- | --- | --- |
| Optional / nilable value types (`T \| Nil`, `T?` sugar) | 📋 planned | Crystal-style **absence** without TypeScript `undefined` juggling; unions, narrowing, and Go lowering—see [optionals RFC hub](./examples/in/rfc/optionals/README.md) ([00](./examples/in/rfc/optionals/00-crystal-inspired-optionals.md)). **Partial today (not this row):** `nil` literal on pointers/`Map`/`Array`/`Error`, `ensure … is Nil()` / `ensure !ident`, and `*T` `.Present()` / `.Nil()` constraints—see [Check a value](./docs/language/check-a-value.mdx). |
| `Result(Success, Failure)` + structured `error` | 🔬 experimental | **`Result(S, F)`** models success or failure as a single value. Use **`ensure`** and **`if x is Ok()`** to branch. Go multi-return in a **single-expression** context becomes a **tuple** (`t.0`, `t.1`)—not an auto-wrapped **`Result`** (see [Go interop & unions RFC](./examples/in/rfc/optionals/01-single-return-unions-and-go-interop.md)). Bridge from Go with **`n, err := …; ensure !err; return n`**; a failed `Error` absence check implicitly propagates `err`. Multi-assignment **`v, err := pkg.F()`** is unchanged. **Still open:** failure types beyond built-in **`Error`**, user **generics** on **`Result`**, full [RFC 02 error story](./examples/in/rfc/errors/02-first-class-errors-normative.md). |
| Structured error system (nominal **`error X { … }`**, **`forst/errors`**, observability) | 🔬 experimental | **Works today:** named **`error X { … }`** types, assignable to built-in **`Error`**, with Go and TypeScript emit. Unions of named errors (`type ErrKind = ParseError \| IoError`) type-check and emit sealed Go interfaces plus TS unions. **Still open:** authoring failures only through **`ensure`**, implicit errors from **`ensure … else`**, tagged **`forst/errors`** runtime, TS **`_tag`** story—see [errors hub](./examples/in/rfc/errors/README.md). Failure keyword: [RFC 12](./examples/in/rfc/refinements/12-accepted-decision.md) replaces **`or`** with **`else`**. |

### Guards, `ensure`, and narrowing

**Intention:** Combine **runtime validation** with **refinement** so constraints and narrowing stay tied to types—developers give the compiler **clear intent** ([PHILOSOPHY](./PHILOSOPHY.md#guiding-principles)), and control flow stays easier to follow than implicit guardrails elsewhere.

| Feature | Status | Notes |
| --- | --- | --- |
| `ensure` statements (basic type assertions) | ✅ done | Validates assertions and optional blocks. **Successor narrowing** after a successful `ensure` works for **simple identifiers** (`ensure x is …` then use `x` on the next lines); **compound paths** (e.g. `ensure req.state is …`) remain deferred (see control-flow narrowing). |
| Shape guards (struct refinement) | ✅ done | Refinement on shapes. |
| `is` operator for `ensure` conditions | ✅ done | **`ensure x is …`** is required syntax (`ensure !ident` means “must not be nil”). The compiler checks that the subject matches the guard and emits runtime checks. Bare **`bool`** / **`is true`** subjects get an **`is True()`** diagnostic; call subjects are rejected so **`ensure`** stays identifier-only. Arbitrary built-in constraint validation beyond that is still limited. |
| Type guards (beyond shape guards) | 🔬 experimental | Top-level **`is (subject T) Name { … }`** declarations parse, type-check, and compile to Go. **Still open:** narrowing in every control-flow position (overlaps **Control-flow type narrowing**). Design: [type guards RFC](./examples/in/rfc/guard/guard.md), [interop](./examples/in/rfc/guard/interop.md). **`ensure` / assertion `or` / typed `else` / type `|`:** [RFC 12](./examples/in/rfc/refinements/12-accepted-decision.md). **Type targets** (`ensure place is Type`): [RFC 14](./examples/in/rfc/refinements/14-type-targets.md). **Mutation / aliasing / Go:** [RFC 13](./examples/in/rfc/refinements/13-refinement-stability.md). Plan: [.plans/analyzable-refinements](./.plans/analyzable-refinements/README.md). |
| `ensure`-scoped immutability | 🚫 not planned | Rejected by [RFC 13](./examples/in/rfc/refinements/13-refinement-stability.md): relevant mutation **drops facts**; it does not lock fields. No borrow checker. Go stays untrusted (invalidate), not an “unsafe mut” mode. |
| Binary type expressions (`A \| B`, `A & B`) | 🔬 experimental | Typedefs can combine types with **`|`** (either) and **`&`** (both). Unions of named error types compile to sealed Go interfaces. **Still open:** general unions, full intersection rules, narrowing everywhere, and TS/Go emit for arbitrary combinations—stick to tested error-union and alias paths for now. **Literal unions:** [RFC 12](./examples/in/rfc/refinements/12-accepted-decision.md). Assertion disjunction is keyword **`or`**, not **`|`**. |
| Control-flow type narrowing | 🔬 experimental | After **`if x is …`**, the compiler treats **`x`** as the narrower type inside that branch. The same idea applies after a successful **`ensure`** for simple variable names. **Still open:** dotted paths like **`req.state`**, and merging types when branches rejoin. See [type guards & optionals](./examples/in/rfc/optionals/10-type-guards-shape-guards-and-optionals.md). |

### Generics, aliases, and nominal features

**Intention:** Reach **parametric abstraction** and naming convenience **without** type-level metaprogramming or hidden coercions—see [No metaprogramming](./PHILOSOPHY.md#no-metaprogramming) and [Anti-features](#anti-features). Forst stays **function-centric** until methods land; aliases and interfaces bridge Go interop gradually.

| Feature | Status | Notes |
| --- | --- | --- |
| Generic types | 📋 planned | User-declared type parameters on types and functions; see [generics RFC](./examples/in/rfc/generics/00-user-generics-and-type-parameters.md). |
| Type aliases | 🔬 experimental | Simple **`type Name = BaseType`** aliases resolve through chains for field access and type compatibility where wired. |
| Methods (`func (t T) M()`) | 📋 planned | Forst is function-centric; no method declarations. |
| Struct embedding (anonymous fields) | ✅ done | Type-only shape fields embed and promote fields like Go; emits anonymous struct fields. |
| Struct tags (user-authored) | ✅ done | Optional backtick or quoted string after a field type; explicit tag emits to Go and overrides auto-`json` when `-export-struct-fields` is on. |
| `interface{ }` satisfaction / embedding | 🔬 experimental | Structural shapes and Go interop differ from Go’s interface model. **Partial:** Go interface **fields** use Go assignability (concrete handlers into **`http.Handler`** slots). Full Forst-side interface satisfaction still open. |
| Type assertions `x.(T)` | ❓ unclear | The implications for Go interoperability and Forst narrowing need investigation before deciding whether to support them. |
| Type switches `switch v := x.(type)` | 🚫 not planned | Use Forst’s explicit `is` / `ensure` narrowing instead. |
| `const` / `iota` | ✅ done | Top-level `const` (single and grouped) with Go `iota` semantics; emits literally to Go. |

### Control flow & statements

**Intention:** Offer **familiar, readable** control flow (Go-shaped **`if`**, **`for`**, **`defer`**, **`go`**) so execution paths stay explicit—consistent with [predictable behavior](./PHILOSOPHY.md#no-unpredictable-behavior) and the anti-feature stance against hidden jumps ([Anti-features](#anti-features)).

| Feature | Status | Notes |
| --- | --- | --- |
| `if` / `else` (incl. init statement) | ✅ done | Same structural forms as Go. |
| `for` loops (infinite, condition-only, three-clause, `range`) | ✅ done | Covers the usual Go forms. **Gaps vs Go:** channel `range`, Go 1.22+ integer `range`. |
| Postfix `++` / `--` | ✅ done | Increment/decrement as standalone statements and in `for` post clauses. |
| Compound assignment (`+=`, `-=`, `*=`, `/=`, `%=`, `&=`, `\|=`) | ✅ done | Parse, typecheck, emit, and **`forst fmt`** round-trip for **`+=`**, **`-=`**, etc. |
| Shift / xor compound assignment (`^=`, `<<=`, `>>=`, `&^=`) | ✅ done | Parse, typecheck, emit, and **`forst fmt`** round-trip for bitwise shift/xor compound forms. |
| Bitwise operators (^, <<, >>, &, \|, &^) | ✅ done | Go-faithful precedence; integer operands only. **`byte`** works with **`Int`** on the other side (result stays **`byte`** when appropriate); index **`byte`** slices with **`Int`**. |
| Integer literals (`0x` / `0o` / `0b`) | ✅ done | Hex, octal, and binary forms parse; **`forst fmt`** and Go emit keep the source spelling (no forced decimal rewrite). |
| `break` / `continue` | ✅ done | Unguarded and **labeled** forms. Labels attach to `for` loops; labeled `break`/`continue` resolve against the enclosing function's loop labels (closures cannot see outer labels). |
| `switch` / `case` / `default` / `fallthrough` | ✅ done | Tag and boolean **`switch`** statements work like Go, including **`fallthrough`**. Type switches (`switch v := x.(type)`) are deliberately unsupported; use `is` narrowing instead. |
| `select` | 📋 planned | Not a Forst keyword yet; needs full statement support (see also channel row below). |
| `defer` / `go` statements | ✅ done | **`go`** starts a goroutine; **`defer`** runs a call when the surrounding function returns—runtime behavior is Go’s. Operand must be a **function or method call** (not e.g. `<-ch`). Parenthesized calls like **`defer (f())`** are rejected. Same predeclared builtins Go forbids as standalone expression statements (`append`, `len`, `make`, …) cannot be deferred or run in a goroutine this way. Anonymous **`go func(){ … }()`** / **`defer func(){ … }()`** work via function literals. |
| Function literals / closures | ✅ done | **`func(params): T { … }`** in expression position; closure over outer variables; **`func() { … }()`** IIFE; **`func(T): U`** function types for parameters. Nested **`return`** uses the closure signature (does not inherit outer **`Result`** lowering). |
| Labeled statements + `goto` | ✅ done | **`goto label`** and **`label:`** on arbitrary statements. Same-function scope (excludes nested func literals); no jump into a block; no jump over a variable declaration that remains in scope at the target; unused labels are errors — matching the Go spec. |

### Builtins, runtime, and platform

**Intention:** Reuse **Go’s predeclared builtins and runtime** where they match backend reality, while **Forst-native** ergonomics (e.g. **`make`/`new` with Forst types**) catch up over time. A first-class **`unsafe`** package is **not** on the roadmap—only reachable through **Go interop** (see [Anti-features](#anti-features)).

| Feature | Status | Notes |
| --- | --- | --- |
| Built-in calls (`make`, `new`, `append`, `copy`, `len`, `cap`, `close`, …) | 🔬 experimental | Go predeclared builtins type-check and emit. **`make`** / **`new`** take a Forst type as the first argument (`make(Array(Int), n)`, `make(map[String]Int)`, `new(Int)`). **`new(imported.T)`** and **`&imported.T{…}`** work for loaded Go named types; methods resolve on the resulting pointer. Other builtins unchanged. |
| Slice subslice expressions (`xs[low:high]`, `xs[low:]`, `xs[:high]`) | 🔬 experimental | Slice bounds syntax on Forst arrays/slices; lowered to Go slice expressions. Distinct from variadic **spread** at Go call sites (see [Go interoperability](#go-interoperability)). |
| Fixed-size arrays `[N]T` | ✅ done | Parses, typechecks with Go assignability rules (not interchangeable with `[]T`), emits Go `[N]T`. |
| Channels (`chan`, `<-`, `range` on channel) | 📋 planned | No end-to-end channel support yet—channel types are not fully modeled, **`select`** is missing, and there is no user-facing syntax to rely on. |
| Build tags, `//go:build`, assembly | 📋 planned | See “Go backwards compatibility” / emit targets. |
| `unsafe` package (via `import "unsafe"` + qualified calls) | 📋 planned | **Not working end-to-end yet:** calls like **`unsafe.Sizeof`** and **`unsafe.Pointer`** are not wired through Forst↔Go typing. **Workaround:** use **`unsafe` in hand-written Go** and call from Forst. **Intended** model: same as Go—**only** the **`unsafe` package**, lexically **`import "unsafe"`** + **`unsafe.*`**—see [Go interoperability](#go-interoperability). |

---

## Go interoperability

**Intention:** Make Forst a **first-class citizen in Go modules**—compile to readable Go, load real Go packages for interop, share types with hand-written Go, and wire **runtime services at entry points** with compile-time obligation checks—supporting [Go interoperability](./PHILOSOPHY.md#go-interoperability) and adoption beside existing backends.

| Feature | Status | Notes |
| --- | --- | --- |
| Transpile to Go (packages, types, functions) | ✅ done | Main compiler output. |
| Runtime validation from type constraints | ✅ done | Checks emitted from types. |
| Providers (`use` / `with`, derived `Providers(f)`) | 🔬 experimental | Separate **request data** from **runtime services** (loggers, clocks, databases). **`use`** declares a need; **`with`** supplies implementations at **`main`**, tests, or a server entry. **Works:** compile-time errors for incomplete wiring; generated Go gets one deduplicated services struct; cross-package modules and tests. **Not yet:** editor polish, stable host discovery format, sidecar wiring patterns, Go-interface edge cases. [User guide](./docs/language/providers.mdx), [RFC](./examples/in/rfc/providers/README.md). |
| `import` of Go packages in Forst | 🔬 experimental | Common paths work; not a full Go loader yet. Import paths that point at **another Forst package** in the same module resolve as Forst siblings, not as hand-written Go stubs. **Also:** `.ft` in `go.mod` dependencies (`github.com/...`, `replace`) are discovered on the import graph, typechecked for their public surface, and emitted via `.forst/overlay/` replaces. |
| Load & typecheck imported Go source | 🔬 experimental | The compiler loads Go packages referenced from `.ft` files using the module’s **`go.mod`**. **`forst run`** / **`forst build`** accept **`-root`** to merge all same-package sources under a tree. Package-dir emit for **`run`/`watch`** is opt-in via **`generate.go`** / **`-o`**. |
| Type-check Forst↔Go calls | 🔬 experimental | Qualified calls **`pkg.Func`** are checked against loaded Go signatures when imports resolve. Primitives, slices, pointers, `error`, and `interface{}` (incl. variadic) are mapped; named Go types, maps, channels, fixed arrays, and function types map through the FFI boundary. Forst **`func`** literals assign to concrete Go **`func`** parameters (e.g. **`http.HandleFunc`**). Generic Go APIs (e.g. **`slices.Contains`**, **`slices.Sort`**) instantiate at call sites via type inference. **Go assignability** applies to interface fields (e.g. **`ServeMux`** into **`http.Server.Handler`**). Arguments that already carry a concrete Go type prefer that type before Forst↔Go mapping. Same-package Forst shape aliases assign into sibling Go structs that expect that type. Crypto constructors such as **`sha256.New`** / **`hmac.New`** type-check as Go function values. **Still open:** remaining Go type-alias identity gaps and some interface edge cases. |
| Named Go types in Forst signatures | 🔬 experimental | Imported Go named types (e.g. `os.File`, `exec.Cmd`) map to qualified Forst type names and emit as Go selectors. In-module Forst↔Go named types already share `pkg.Type` when the package is in the same module. Construction: **`new(imported.T)`**, **`&imported.T{…}`**, then methods on the pointer. |
| Go composite types (maps, channels, arrays, funcs) | 🔬 experimental | `map[K]V`, `[N]T`, `chan T`, and `func` types map through the FFI boundary where Forst has matching type syntax. Exported-field anonymous Go structs map to Forst shapes; embedded or unexported fields stay opaque. Go type aliases still have identity gaps. |
| Variadic spread in Go call arguments (`expr[low:]...`) | 🔬 experimental | Spread a Forst slice subslice into a Go variadic parameter (e.g. `exec.Command(argv[0], argv[1:]...)`). |
| Field and method access on Go values | 🔬 experimental | After a Go import or qualified call, dotted fields and method chains on Go-typed values work (e.g. `cmd.ProcessState.ExitCode()`). |
| Same-package unexported Go symbols | 🔬 experimental | Mixed `.ft` + `.go` in one package can call unexported Go funcs and access unexported fields when the Go source is co-located. **`forst run`** and **`forst test`** sandboxes link sibling **`.go`** (and shim Go-only deps). **`forst run`** / **`watch`** emit beside the entry package when **`generate.go`** (or **`-o`**) is set so handwritten same-package Go stays co-located. |
| `go/packages` load error reporting | 🔬 experimental | Failed package loads surface as `go-import` diagnostics instead of silently skipping FFI checks. |
| Match Go idioms where it matters (`error`, naming) | 🔬 experimental | Iterative polish; conventions still evolving. |
| Expose Forst functions to non-Forst callers (HTTP, RPC, subprocess) from **generated Go** | 🔬 experimental | Compose servers in Go; Forst-native handler patterns not in place yet. |
| Generic Go API consumption (`slices.Contains`, `maps.Clone`, …) | 🔬 experimental | Type inference and `types.Instantiate` at call sites; multi-parameter generics (e.g. `S ~[]E, E comparable`). |
| User-defined generic functions (`func f[T any](...)`) | 🔬 experimental | Scoped type parameters, call-site inference, explicit type args (`f[Int](x)`), `any`/`comparable` constraints, and Go 1.18+ generic func emission. |

---

## Go backwards compatibility

**Story:** Forst emits Go so teams can adopt it beside hand-written Go in the same module and rely on the standard toolchain. Parity here means **generated code that compiles and behaves predictably**, **sensible defaults for which Go version we assume**, and **explicit knobs** when output must run on older toolchains.

**Intention:** Keep **trust in the Go toolchain** (build tags, formatting, version targets) so generated code stays boring and reviewable—aligned with predictable deployment and [Go interoperability](./PHILOSOPHY.md#go-interoperability).

| Feature | Status | Notes |
| --- | --- | --- |
| Emitted code is valid Go for the **supported compiler/toolchain** (see `forst/go.mod`) | ✅ done | Primary guarantee today: output matches what we build and test against. |
| Forst syntax that mirrors Go (subset) maps to familiar Go constructs | ✅ done | Includes `for`/`range`/`break`/`continue`, `if` with init, and boolean literals as keywords—see README. |
| Mixed packages: Forst (`.ft`) alongside `.go` in one module / tree | 🔬 experimental | Common layouts work: same-package hand-written Go, multi-package **`forst test`**, and opt-in package-dir emit for **`run`/`watch`**. Remaining edge cases still shake out with imports and discovery. |
| Idiomatic, readable generated Go (names, structure, `error` handling) | 🔬 experimental | Ongoing polish; not frozen. |
| **Selectable minimum Go version** for emitted code (e.g. emit for older `go` than the compiler) | 📋 planned | Single emit path; no `-target` / compatibility mode yet. |
| Emit avoids language/stdlib features newer than chosen target (when targets exist) | 📋 planned | Depends on versioned emit; not in place until targets exist. |
| Output routinely **`gofmt`-clean** (or documented exceptions) | 🔬 experimental | Aim for gofmt-friendly layout; not asserted everywhere in tests yet. |
| Policy for **stdlib / API deprecation** in generated code as Go releases ship | 🔬 experimental | Track Go release notes over time; no separate audit pipeline yet. |
| Build tags, file splits, or shims for **per-version or per-GOOS** generated code | 📋 planned | Not implemented. |

---

## TypeScript interoperability

**Intention:** Give **Node/TS clients accurate types** and a **smooth dev loop** (`forst generate`, `forst dev`, sidecar) for full-stack workflows—per [TypeScript interoperability](./PHILOSOPHY.md#typescript-interoperability) and gradual adoption without abandoning the TS ecosystem.

| Feature | Status | Notes |
| --- | --- | --- |
| Declaration emit (`.d.ts` / TS types from Forst) | ✅ done | `forst generate` and TS transformer; contract outline: [03-forst-generate-contract.md](./examples/in/rfc/typescript-client/03-forst-generate-contract.md). |
| TypeScript emit excludes Provider wiring (data-only client surface) | ✅ done | Functions with unsatisfied **`Providers(f)`** are omitted from TS exports; sidecar wire stays payload-only ([ADR-020](./examples/in/rfc/providers/ADR.md#adr-020-typescript-emit-excludes-providers-concept), [ADR-021](./examples/in/rfc/providers/ADR.md#adr-021-runnable-exports-only-when-providersf-is-empty)). Generate prints a Warn report (package, function, reason). Go-side discovery JSON may expose inferred needs for host authors — not the TS client artifact. |
| Merge outputs across `.ft` files | ✅ done | Shared `types.d.ts`; duplicate handling. |
| Generated `@forst/gen` client (ephemeral `.forst/client`, subpaths, inlined transport) | ✅ done | Default outDir `.forst/client` (gitignored), linked into `node_modules` as `@forst/gen` / `@forst/gen/<pkg>`. Committed mode via `generate.outDir` + `link: never`. Requires a lifecycle script such as `postinstall`. Plan: [.plans/client-ergonomics](./.plans/client-ergonomics/SPEC.md). |
| Typed invoke errors, `.safe()`, call options | ✅ done | Tagged `_tag` errors, `isInvokeFailure`, per-call `signal` / `timeoutMs` / `retries`. |
| `@forst/gen/$testing` test scopes | ✅ done | `withForstTestScope` function / package / transport overrides without HTTP or module mocks (`generate.testingSubpath` override for custom `$…` keys). |
| Effect mode (`generate.effect`) | ✅ done | Optional Effect-returning packages, `ForstClientLive` / layers, Effect-shaped errors in Promise mode without an `effect` dependency. Peer `effect >= 3.17.0` when enabled. |
| `forst generate --watch` + sidecar `watchGenerate` | ✅ done | Regenerates the client on `.ft` edits so editor types stay in step. |
| `forst dev` HTTP API + JSON contract | ✅ done | Endpoints `/health`, `/functions`, `/invoke`, `/types`, `/version` (includes **`contractVersion`** for HTTP API compatibility). Spec: [02-forst-dev-http-contract.md](./examples/in/rfc/typescript-client/02-forst-dev-http-contract.md). [`@forst/sidecar`](./packages/sidecar/README.md) reference client; `bun test` in `packages/sidecar`. **Shipped today:** **JSON** request/response bodies over HTTP. **Future:** **protobuf**-based RPC—see **Protobuf sidecar wire** row below. |
| **Protobuf** sidecar wire (gRPC / Connect) | 📋 planned | **Protocol Buffers** as the **contract-first** IDL for a high-throughput TS ↔ Forst boundary, carried as **gRPC** (HTTP/2) or **Connect** (protobuf over HTTP)—see [sidecar wire format](./examples/in/rfc/sidecar/11-wire-format.md). Not implemented; MVP remains JSON (**`forst dev` HTTP API** above). |
| Native ES modules / Node addon path | 📋 planned | Design in [Forst as native ES modules](./examples/in/rfc/esm/README.md) (Forst → Go → addon → ESM); no compiler or addon pipeline yet. Complementary to HTTP sidecar + `forst generate`, not a replacement. |
| `forst generate` + `ftconfig` discovery | ✅ done | **`forst generate`** accepts **`-config`**, loads config from the target tree when omitted, and uses the same **include/exclude** discovery as **`forst dev`**. |
| Generate plugins (semantic emitters on `forst generate`) | 🔬 experimental | Configured binaries receive a typechecked snapshot and write contracts or handlers (JSON Schema, oRPC, file routes, React Router). Protocol v1. No independent plugin semver. Docs: [Plugins](./docs/workflow/plugins.mdx). |
| `@forst/sidecar` on npm and JSR | 🔬 experimental | Package metadata (`package.json`, `jsr.json`) and [publish-packages.yml](./.github/workflows/publish-packages.yml) (sidecar jobs); first registry publish is a maintainer step. |
| `@forst/cli` on npm and JSR (compiler / CLI) | 🔬 experimental | [`@forst/cli`](./packages/cli/README.md): Release Please `cli-v*` tags; lazy-download of the native `forst` from GitHub Releases; npm + JSR in [publish-packages.yml](./.github/workflows/publish-packages.yml) (CLI jobs). Compiler binaries still ship on root `v*` ([release.yml](./.github/workflows/release.yml)). Official plugin installs fail loudly when release assets are missing (no soft-ignore of 404s). Short overview: [README — npm](./README.md#npm). Not the same package as sidecar. |
| Run compiler or sidecar from Node.js | 🔬 experimental | **`@forst/cli`** downloads the native compiler; **`@forst/sidecar`** wraps the dev server—both on npm / JSR. Monorepo dev path unchanged. |
| Dev experience: watch + HTTP types (where applicable) | ✅ done | **`@forst/sidecar`** + **`forst dev`**: file watch, debounced type generation (default on in development), configurable roots—see [`packages/sidecar/README.md`](./packages/sidecar/README.md). |
| **Invocation:** stable contract from Node/TS to **running** Forst (`forst dev`) | ✅ done | HTTP **`POST /invoke`** with a JSON envelope; the sidecar client surfaces typed errors and checks API/compiler version compatibility on connect. See [01-integration-profiles.md](./examples/in/rfc/typescript-client/01-integration-profiles.md). |
| **Route- or module-level Forst** (handlers + client types in one arc) | 📋 planned | Full-stack slices—not only `.d.ts` for hand-written TS handlers—not implemented. |
| OpenAPI / JSON Schema from shapes; pluggable transport (IPC, stdio); WASM | 📋 planned | JSON Schema and related **generate plugins** exist experimentally (see **Generate plugins** above). Remaining: other transports (IPC, stdio) and WASM. In-process **native** integration is tracked separately (see **Native ES modules / Node addon path** above). |
| CI: sidecar against downloaded compiler | 🔬 experimental | May fail until a release ships a **`forst`** binary with a compatible **`dev`** subcommand; local sidecar tests are the CI bar today. |
| **JS bridge interop** (Forst → TS at runtime) | 🔬 experimental | Opt-in **`import "./path" js`** lets Forst call JavaScript/TypeScript at runtime: sync **`call`**, async **`callAsync`**, and generator-style pull over a small RPC bridge. Docs: [Call JavaScript from Forst](./docs/interop/bridge.mdx). **Still open:** published npm/Go module story, CI guard flags, observability, sandboxing—see [bridge-interop RFC hub](./examples/in/rfc/bridge-interop/README.md). |

**See also:** [README (npm)](./README.md#npm), [`packages/cli/README.md`](./packages/cli/README.md), [examples/in/rfc/typescript-client/README.md](./examples/in/rfc/typescript-client/README.md) (RFC index), [examples/in/rfc/sidecar/00-sidecar.md](./examples/in/rfc/sidecar/00-sidecar.md), [examples/in/rfc/sidecar/11-wire-format.md](./examples/in/rfc/sidecar/11-wire-format.md) (protobuf, gRPC, Connect), [examples/in/rfc/sidecar/tests](./examples/in/rfc/sidecar/tests), [examples/client-integration/README.md](./examples/client-integration/README.md).

---

## Tooling & developer experience

**Intention:** Turn the compiler into **actionable feedback** in the editor—diagnostics, navigation, refactor—matching [clear, actionable development feedback](./PHILOSOPHY.md#guiding-principles) and fast iteration.

**LSP (`forst lsp`):** The server speaks **JSON-RPC over HTTP** (`POST /` on the listener port), not stdio—editors need a small bridge (the in-repo VS Code extension provides one). On startup it advertises completion, hover, diagnostics, go-to-definition, find references, rename, formatting, code actions, symbols, folding, and code lens. **Navigation:** top-level symbols can resolve across **same-package** `.ft` files open in one directory; locals and parameters stay scoped to their binding. Formatting may return nothing when the buffer cannot be formatted.

| Feature | Status | Notes |
| --- | --- | --- |
| LSP: HTTP transport & process (`forst lsp`) | ✅ done | JSON-RPC on **`POST /`**; **`GET /health`** for health checks. |
| LSP: `initialize` / `serverInfo` | ✅ done | Advertises text sync, completion (trigger **`.`**, **`:`**, **`(`**, space), hover, diagnostics, definition/references, rename (with prepare), document formatting, code actions, document + workspace symbols, folding, code lens. Server name **`forst-lsp`**; version matches the compiler. |
| LSP: lifecycle (`shutdown`, `exit`) | ✅ done | Clean shutdown handshake. |
| LSP: text document sync | ✅ done | Open, change, and close events keep an in-memory copy of each buffer and recompile on edit. |
| LSP: diagnostics | ✅ done | Parse and typecheck on each update; compiler errors appear as editor diagnostics. |
| LSP: hover (`textDocument/hover`) | ✅ done | Shows function signatures (with doc comments from same-package peers when relevant), type definitions, type guards, and inferred variable types. Unary **`&`** hovers as address-of (not bitwise AND) when the previous token disambiguates. **Providers:** **`with`** scope and derived needs on function hovers. **Go imports:** qualified names like **`fmt.Println`** and import paths show Go signatures and docs when loading succeeds; hand-written **`.go`** in the same package too. **Limits:** parse failures fall back to keyword/identifier hovers; some module layouts block Go hovers. |
| LSP: completion (`textDocument/completion`) | ✅ done | Keywords, same-file and same-package symbols, locals/parameters, member names after **`.`**, and exported names after **`pkg.`** (Go packages). Marks the list incomplete when other open buffers might add symbols. |
| LSP: go to definition | ✅ done | Jumps to top-level **func**, **type**, type guards, parameters, and local bindings. Can target another same-package **`.ft`** when package merge applies. **Limits:** destructured params and some nested control-flow edge cases. |
| LSP: find references | ✅ done | Lists occurrences that share the same binding as the cursor, including merged same-package peers. Respects include-declaration. Same limits as go-to-definition for tricky locals. |
| LSP: rename | ✅ done | Renames locals, parameters, and merged-package symbols; skips internal hash-generated type names (`T_*`). |
| LSP: document symbols | ✅ done | Flat outline of top-level **func**, **type**, and type guard symbols in the current file. |
| LSP: workspace symbol | ✅ done | Searches **open** `.ft` buffers only (case-insensitive substring on name). No full-disk index yet. |
| LSP: formatting (`textDocument/formatting`) | 🔬 experimental | Same pipeline as the **Format document** code action; may return nothing when formatting does not apply. |
| LSP: folding (`textDocument/foldingRange`) | ✅ done | Folds function bodies, type definitions, and type guard bodies by brace regions. |
| LSP: code actions & code lens | 🔬 experimental | **Format document** code action when allowed. Code lens is advertised but still minimal. |
| LSP: custom compiler / debug methods | 🔬 experimental | Extra methods expose compiler state for tooling workflows—not general editor features. |
| LSP: protocol & client quirks | 🔬 experimental | **HTTP** transport is non-standard for LSP; stdio clients won’t work without an adapter. Methods not handled by the switch (e.g. some client notifications) get **-32601 Method not found**. |
| Error messages (line numbers, suggestions) | 🔬 experimental | Incremental improvements; quality and completeness still vary. |
| `forst test` (Go-native `Test*` in `*_test.ft` and native Go `_test.go`) | 🔬 experimental | Finds **`Test*`** functions in **`*_test.ft`**, generates temporary Go test code, and runs **`go test`**. Also discovers and runs native Go-only test packages (`*_test.go`). Sandboxes include sibling **`.go`** and Go-only deps. Primary use: Providers wiring, mixed Forst+Go packages, and cross-package integration. Docs: [CLI — `forst test`](./docs/workflow/cli.mdx), [forst-test RFC hub](./examples/in/rfc/forst-test/README.md). |
| More real-world examples | 🔬 experimental | Broader coverage still wanted; a dedicated showcase repo still open. |
| VS Code extension | 🔬 experimental | In-repo **`packages/vscode-forst`**: `.ft` language + grammar, HTTP LSP client, and language providers; **outline**, **folding** (when the server returns ranges), **go to definition**, **find references**, **rename**, **format document** / **format** code action, **workspace symbol** (open files) when the server resolves symbols. Finds **`go`** when the GUI PATH is thin (common toolchain dirs + optional **`forst.go.path`**). **Status bar:** LSP port + connection cue (idle / ready / error), click or **Forst: Focus output** opens the log. **Releases:** `vscode-forst-v*` + [publish-vscode-extension.yml](./.github/workflows/publish-vscode-extension.yml) (not tied to compiler `v*`). **Marketplace** / discoverability still open. |

---

## Docs & community

**Intention:** Lower the **onboarding bar** and grow a **contributor-friendly** project—supporting [Adoption](./PHILOSOPHY.md#adoption) and sustainable community growth around the language.

| Feature | Status | Notes |
| --- | --- | --- |
| Public docs site (Mintlify) | 🔬 experimental | [`docs/`](./docs/) — [Quickstart](./docs/quickstart.mdx), language guides, Go/TS interop, workflow, and [simplified roadmap](./docs/resources/roadmap.mdx). Deployed via Mintlify; validate with `npx mintlify broken-links` from `docs/`. |
| “Getting Started” / onboarding path | 🔬 experimental | Quickstart + installation pages exist; README remains a secondary entry. Broader tutorials and migration guides still open. |
| Example project showcasing core features | 🔬 experimental | `examples/` today; a dedicated showcase repo still open. |
| Contributing guide (`CONTRIBUTING.md`) | 📋 planned | [`docs/README.md`](./docs/README.md) covers doc contributions; full dev-setup contributor guide still open. |

---

## Infrastructure

**Intention:** Keep **main green**, releases **repeatable**, and tests **signal real regressions**—so language and tooling changes stay safe to ship at velocity.

### CI & releases

**Intention:** Automate verification and shipping so every merge stays **releasable** and consumers get **versioned artifacts** predictably.

| Feature | Status | Notes |
| --- | --- | --- |
| CI pipeline (lint, tests, compiler build) | ✅ done | GitHub Actions; `task ci:test` (coverage) and `task ci:e2e` (E2E). |
| Release automation | ✅ done | release-please + git tags. |

### Tests and merge gate

**Intention:** Prefer **targeted tests** on compiler hot paths over broad, low-signal suites—so refactors and RFC-sized changes stay [safe to reason about](./PHILOSOPHY.md#guiding-principles) in review.

| Feature | Status | Notes |
| --- | --- | --- |
| Unit / library tests (Go) | ✅ done | **`go test`** across the compiler and libraries (optional **`-cover`** locally); Coveralls upload in CI. |
| Integration & example runs in CI | ✅ done | Same pipeline runs example tasks (e.g. sidecar) after unit tests. |
| Colocated Go tests (`foo.go` + `foo_test.go`) | ⏳ in progress | New and touched production code should gain stem-matched tests beside the source. LSP, module loading, and Go hover paths are prioritized first. |
| Merged statement totals on hot paths (LSP, module loading, code generation) | ⏳ in progress | Coverage targets for compiler hot paths—see [merged statement plan](./docs/adoption/merged-statement-plan.md). Do not raise CI coverage gates until the target is met. |
| Validation codegen (`TestEmitValidation_*`) | 🔬 experimental | Tests assert that generated Go matches expected output for built-in constraints and type guards. Still expanding over time. |
