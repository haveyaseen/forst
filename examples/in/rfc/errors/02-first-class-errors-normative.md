# First-class errors (normative RFC)

**Audience:** Language designers, compiler implementers, and RFC authors. **Status:** **Normative target** for Forst **nominal errors**, **`ensure`-only failure** for **`Result`**, **`or`** typing, and **failure-type inference**—aligned with the project’s recorded strategic decisions for first-class errors.

**Depends on:** [00 — Error system architecture](./00-error-system-architecture.md) (product sketch, Go `forst/errors`, TS compatibility notes), [01 — Ensure-only failure propagation](./01-ensure-only-failure-returns.md), [optionals / `Result` and error types](../optionals/02-result-and-error-types.md), [09 — `ensure` and narrowing](../optionals/09-ensure-is-narrowing-and-binary-types.md), [12 — Result primitives](../optionals/12-result-primitives-without-ok-err.md).

**Supersedes for `Result` failure authoring:** [12 §5.7](../optionals/12-result-primitives-without-ok-err.md) **nominal `return` failure** is **not** the normative spelling once this RFC is implemented—**failure** must go through **`ensure … or`** (see §2). Success remains **`return x`** with **`x : S`** where [12](../optionals/12-result-primitives-without-ok-err.md) applies.

---

## 1. Goals and non-goals

### Goals

| Goal | Normative intent |
| --- | --- |
| **Ensure-only failure** | In functions returning **`Result(S, F)`**, **domain failure** is introduced only via **`ensure <condition> or <failure>`**, not via **`return SomeError(…)`**. |
| **Nominal errors** | Concrete failure kinds are **named** with **`error X { … }`** (each **implicitly** relates to the language base **`Error`** for typing and lowering—there is **no** `error Error {}` in source); see [00](./00-error-system-architecture.md#error-types-base-only-in-the-language). |
| **Constructors** | Failures use Go-style struct literals: **`N{}`** (zero payload) or **`N{ field: … }`** (keyed fields); see §4. |
| **`Result` guards** | **`Ok` / `Err`** remain **guards** on **`Result`** for **`is` / `ensure`** ([12 §0](../optionals/12-result-primitives-without-ok-err.md#0-scope-split-guards-vs-constructors)); failure **payload** is carried as **`Err(NominalValue)`** with **nominal** typing (§5). |
| **`or` typing** | The **`or`** arm is always a **nominal error** value assignable to **`F`**; multiple arms use **LUB** (§6). |
| **Inference** | When **`F`** is omitted from the signature, infer **`F`** from the **LUB** of **`or`** arms; **inferred** nominals are **exported** (§6). |
| **Optional messages** | No required human-readable **`message`** on base **`Error`** ([00](./00-error-system-architecture.md)). |

### Non-goals (v1)

| Non-goal | Pointer |
| --- | --- |
| **Error mapping / wrapping syntax** | §9 — deferred; no `mapError`-style surface in v1. |
| **Built-in catalogue** | No standard-library error variants like `ValidationError`; the abstract base **`Error`** exists for typing and lowering but is **not** written as **`error Error {}`** ([00](./00-error-system-architecture.md)). |
| **User–user inheritance** | v1 has **no** inheritance between nominal errors (**no** **`extends`**, **no** subtyping lattice among user nominals beyond each nominal being compatible with **`Error`** / **`F`** as declared). |
| **Mandating Effect** | TS emit is **compatible** with Effect-style **`_tag`** / **`cause`** where practical; Effect is not required ([00 — Phase 3](./00-error-system-architecture.md#phase-3-typescript--client-integration-weeks-5-6)). |

---

## 2. Raising failures: `ensure` only (`Result`)

**Rule:** In a function whose return type is **`Result(S, F)`** (or the lowered equivalent), **every failure exit** must be expressible as **`ensure … or <nominal failure>`**. A plain **`return`** of a failure value where that value is typed as **`F`** is **not** normative for v1—authors use **`ensure`** (and nominal constructor on **`or`**) instead.

**Rationale:** Keeps one primary failure mechanism; pairs with **positive framing** (state **what must hold**, then name the failure on **`or`**) as in [00 — Raising errors](./00-error-system-architecture.md#raising-errors-ensure).

**Relationship to [01](./01-ensure-only-failure-returns.md):** Same **ensure-first propagation** for **`Result`** unwrap-or-fail; this RFC **narrows** failure **authoring** to **`ensure`** only, which [01](./01-ensure-only-failure-returns.md) motivates but does not fully spell as a ban on **`return` nominal failure**.

**Relationship to [12 §5.7](../optionals/12-result-primitives-without-ok-err.md):** Where §5.7 allows **nominal `return`** failure, **this RFC overrides** for **`Result(S,F)`** functions: success **`return x`**, failure **`ensure … or`**.

---

## 3. Nominal error types (explicit and implicit)

### 3.1 Language base (not a user declaration)

The typechecker and runtime treat a single abstract **base `Error`** (fields and lowering per [00](./00-error-system-architecture.md)). **Authors must not write `error Error { … }`**—it does not appear in normative Forst source.

### 3.2 Declaring a nominal: `error X { … }`

Every user error is introduced as **`error X { … }`**. The language ties that nominal to the abstract base **`Error`** for typing and lowering; there is **no** separate **`extends`** syntax and **no** inheritance between **two** user-defined error nominals—if you need a “more specific” failure, declare **another** nominal (and a distinct constructor) rather than refining an existing one through inheritance.

```ft
error NotPositive {
    field: String
}
```

### 3.3 Implicit declaration from `ensure`

If **`N`** is **not** yet declared, the **first** use of a constructor on the **`or`** side **introduces** **`error N { … }`** whose **payload shape** is inferred from that constructor (typed compatibly with **`Error`** / **`F`** per §5). Example:

```ft
ensure n > 0 or NotPositive{
    field: "n",
}
```

If **`NotPositive`** was not declared earlier, the compiler records **`error NotPositive { field: String }`** (field names and types must stay **consistent** across the package).

### 3.4 Constructor forms

- **Zero-argument:** **`RateLimited{}`** — empty or unit payload per language rules.
- **Single shape:** **`N{ f1: v1, … }`** — one record; fields define the nominal payload (0 or 1 shape argument per product decision).

---

## 4. Triple: type, constructor, and `Result` guard (`Err(Nominal)`)

Each nominal error provides:

1. **Type** — **`error N { … }`** (explicit or implicit §3.3).
2. **Constructor** — **`N{}`** or **`N{ … }`**.
3. **Guard on `Result`** — Do **not** invent a separate synthetic guard per nominal on **`Result`**. Use the unified **`Err(…)`** guard whose **payload** is **nominally typed**:

   - Narrowing uses **`is Err(payload)`** / **`ensure r is Ok() or …`** such that the **failure branch** carries **`N`**’s payload type.
   - Mental model: **`Err(NominalValue)`** — programmers pass an **explicit nominal** value; **`Err`** is not a generic opaque wrapper at the **type** level.

**Built-in `Ok` / `Err`:** Remain **discriminants** for **`Result`** in **`is` / `ensure`** ([12 §0](../optionals/12-result-primitives-without-ok-err.md#0-scope-split-guards-vs-constructors)); they are **not** value constructors at **`return`** sites.

---

## 5. `ensure … or` static typing

Let **`T(e)`** denote the static type of expression **`e`**.

1. **`or`** **must** be a **nominal error** expression: **`N{}`** or **`N{ … }`** with **`N`** a user-defined nominal (compatible with the language **`Error`** model and assignable to **`F`** where required).
2. For **`Result(S, F)`**: **`T(orExpr)`** must be assignable to **`F`** (same width / assignability story as [02 — Result and error types](../optionals/02-result-and-error-types.md) §2; there is **no** user–user inheritance chain to “walk”).
3. **LUB across arms:** If a function contains several **`ensure … or Eᵢ(…)`**, define **`F_or = lub(E₁, …, Eₙ)`** over the declared failure types (typically **`Error`** if arms use **distinct** nominals with no finer common **`F`**). The **declared** **`F`** must satisfy **`F_or <: F`** (typically **`F`** is **`F_or`** or a supertype such as **`Error`**).

**Note:** If **`F`** is **`Error`**, any user nominal used on **`or`** is typically assignable; APIs that want a **narrow** failure set should declare **`F`** as that nominal (or a **single** nominal shared by all arms).

---

## 6. Inferring `F` from `ensure` (option I + export rule)

### 6.1 When `F` is omitted

If the signature omits **`F`** or uses a placeholder the compiler treats as “infer,” compute **`F`** as the **least upper bound** of all **`T(orExpr)`** from **`ensure … or`** in the function body (§5). If there is **no** **`ensure`**, **`F`** inference is **unspecified** in v1 unless another rule applies (e.g. **error** diagnostic or default **`Error`**—implementation choice to document in compiler notes).

### 6.2 Export rule

Any **nominal error** type that appears in an **inferred** **`F`** of a **public** API (or that is needed for **narrowing** by consumers) must be **exported** in Forst; **Go** and **TypeScript** emit must expose corresponding types (**exported** identifiers / modules) so callers can use **`errors.As`**, **`is`**, or tagged unions. This avoids “hidden” inferred failures on exported surfaces.

---

## 7. Out of scope (v1): mapping and wrapping

The following are **not** part of the v1 language surface; they may be revisited in a **future** RFC:

- **`mapError`**, **`catchTag`**, or **pipe**-style **error** transforms as **language** syntax.
- **`ensure … or`** expressions that **remap** an inner **`Err`** payload into a **different** nominal in one step (beyond **forwarding** the same nominal or **`ensure r is Ok() or …`**).
- Cross-nominal **coercion** tables at **module** boundaries beyond normal assignability of **`F`**.

**Interim:** Handlers may use **`if` / `switch`** on narrowed **`Err`** payloads in **recovery** paths ([01](./01-ensure-only-failure-returns.md) allows **`if x is Err`** for **success** recovery); **propagation** stays on **`ensure`**.

---

## 8. Document map

| Topic | Where |
| --- | --- |
| Architecture sketch, `forst/errors`, TS / Effect **compatibility** | [00](./00-error-system-architecture.md) |
| **Ensure** vs **`if`** for propagation | [01](./01-ensure-only-failure-returns.md) |
| **`Result(S,F)`** error-kinded **`F`** | [02 optionals](../optionals/02-result-and-error-types.md) |
| **Guards vs constructors**, **`return x`** success | [12](../optionals/12-result-primitives-without-ok-err.md) |
| **Normative** nominal + **`ensure` + `or` + LUB + inference** | **this document** |

---

## 9. Implementation checklist (compiler)

### Shipped (partial)

- [x] **Syntax & AST:** **`error X { … }`** parses to **`TypeDefErrorExpr`** (payload shape); **`error Error { … }`** is rejected. Empty payload **`{}`** allowed.
- [x] **Typechecker:** Registers nominal errors; **`N <: Error`** for assignability; structural/hash pipelines use **`PayloadShape`** (shape + nominal error payloads).
- [x] **Emit:** Go lowers payload to a **`struct`** type named **`X`**; TS **`forst generate`** maps nominal errors to interfaces from payload.
- [x] **Tooling:** **`forst fmt`** preserves **`error X { … }`** (not `type X = error …`); LSP hover shows **nominal error** + full definition + leading **`//`** docs (see roadmap).

### Remaining (normative bar)

- [ ] Enforce **ensure-only** failure for **`Result(S,F)`** (diagnostics for **`return` nominal failure** where §2 applies).
- [ ] **Implicit `error N`** from first **`ensure … or N{ … }`**; unify payload shapes across the package.
- [ ] Typecheck **`or`** arms: **`T(or) <: F`**, **LUB** for **`F`** inference (§5–6).
- [ ] **Export** inferred nominals on public **`F`** (§6.2).
- [ ] Lower to Go **`error`** + **`forst/errors`** tags; generate TS types with **`forstTag` / `_tag`** per [00](./00-error-system-architecture.md).

**Tests / examples:** [`error_typedef_test.go`](../../../../forst/internal/parser/error_typedef_test.go), [`error_nominal_test.go`](../../../../forst/internal/typechecker/error_nominal_test.go), [`examples/in/nominal_error.ft`](../../../../examples/in/nominal_error.ft) (`task example:nominal-error`).

---

## Document history

| Change | Notes |
| --- | --- |
| Initial | Normative consolidation: triple **`Err(Nominal)`**, **`or`** **LUB**, inference **I** + export, **§7** deferred mapping, supersede **12 §5.7** failure **`return`** for **`Result`**. |
| Syntax | No **`error Error {}`**; no **`extends`** between nominals. **`error X { … }`** only. |
| Flat nominals | No inheritance between user error nominals; no **`extends`** keyword. |
| §9 checklist | Split **shipped** vs **remaining**; drop stale **`ErrorDecl`** progress; link to `nominal_error.ft` example. |
