# 03 — `else` is failure; `or` is assertion disjunction

**Status:** Syntax and call-site `ensure`. Binding for [12](./12-accepted-decision.md) §§3–6.

The filename `03-or-vs-pipe.md` is historical. **`or` is assertion alternative. `else` is typed failure. `|` is type union, not `is`-assertion disjunction.**

---

## 1. The collision (why `or` was dropped *as failure*)

Spoken English: “ensure this **or** that.” Old Forst English: “ensure this **or** *throw that error*.”

```ft
ensure x is Strong() or TooWeak()   // not failure; two alternatives if both are constraints
ensure x is Strong() else TooWeak() // typed failure
```

`TooWeak()` as an **error constructor** is not a second predicate. Parser today still treats `or` after `ensure` as failure ([`ensure.go`](../../../../forst/internal/parser/ensure.go)). [12](./12-accepted-decision.md) splits the keywords.

**Accepted:**

```ft
ensure x is Strong() else TooWeak()
ensure phone is HasPrefix("+") or HasPrefix("0") else BadPhone()
```

`else` is control flow. `or` is assertion Join. Do not keep dual failure syntax (`or` as error).

## 2. Grammar (normative target)

```text
ensureStmt     ::= "ensure" place "is" target failure
                 | "ensure" "!" ident [failureBlock]
target         ::= typeTarget | assertion
typeTarget     ::= TypeName                 // no parentheses ([14](./14-type-targets.md))
failure        ::= "else" errorExpr
                 | failureBlock
                 | /* contextual default */
failureBlock   ::= "{" statements "}"
assertion      ::= constraintChain { "or" constraintChain }
constraintChain ::= atom { "." atom }
atom           ::= ConstraintCall   // always parentheses: Min(3), Pending()
place          ::= ident { "." ident }   // stable paths only
```

`else` and `failureBlock` are **mutually exclusive** ([12 §13](./12-accepted-decision.md)).

Dot stays **tighter** than `or` (Meet before Join). Failure `else` is a separate clause.

```ft
ensure phone is HasPrefix("+") or HasPrefix("0") else BadPhone()
```

`|` is **not** in this production. `|` is typedef union ([12 §6](./12-accepted-decision.md)).

**Invalid:**

```ft
ensure x == "a" || x == "b" else err          // not an assertion
ensure x is Foo() | Bar()                     // `|` is types, not assertion Join
ensure x is Foo() { log("no") } else Err()    // block XOR else
ensure x is Foo() | bar()                     // `|` is not boolean OR
ensure getUser() is Active()                  // subject is not a place
ensure value is A() or foo() == true else E() // alternative is not a constraint
```

## 3. Why `ensure` must not grow a boolean language

[01](./01-existing-commitments.md): `ensure` tests an **assertion**, not an expression. That is what makes successor narrowing a lookup of `InferAssertionType` instead of WP.

A boolean `ensure` (`ensure cond`) reopens:

- What is the refined type if `cond` is `x > 0 && y.isReady()`?
- Which identifiers narrow?
- How does this interact with `else Err()`?

Forst already has `if` for booleans. `ensure` is the **check + narrow + fail** form of `is`. Keep it that way. Disjunction lives **in the assertion**, where Join is defined.

`ensure x is True()` on a `Bool` stays the way to require a boolean **value**, not a way to sneak in an arbitrary condition.

`|` is **not** general boolean OR ([12 §6](./12-accepted-decision.md)).

## 4. Call-site patterns after this RFC

**Named set (enum):**

```ft
type GameStatus = "playing" | "x_won" | "o_won" | "draw"

func apply(status String): Result(GameStatus, BadStatus) {
  ensure status is GameStatus else BadStatus{}
  return status
}
```

**Inline Join (no extra name):**

```ft
ensure op is HasPrefix("sum") or HasPrefix("avg") else InvalidInput()
```

Also valid on `if`:

```ft
if status is Pending() or Processing() {
  ...
}
```

Prefer a named type when the set is reused. Inline `or` is for local, small, same-subject predicates.

**Still conjunctive playlist in functions:**

```ft
ensure n is GreaterThan(0) else NotPositive{}
ensure n is LessThan(100) else TooBig{}
```

Two failures, two nominal errors, LUB of `F` as [errors 02](../errors/02-first-class-errors-normative.md). **Do not** write `ensure n is GreaterThan(0) or LessThan(100)` unless you mean the **union** of those refinements (a number that is `> 0` **or** `< 100`, i.e. almost everything). That is the teaching moment: **`or` is or of predicates, not “and also check.”** Sequential `ensure` is and.

**Failure block (not in type guards):**

```ft
ensure config is Valid() {
  println("Invalid configuration")
}
```

## 5. Type-guard `ensure` still has no typed failure

Unchanged TG-3. A guard cannot fail with an error value; it can only make `φ` false. Disjunction inside a guard is assertion `or` or an `if` chain ([04](./04-guard-bodies.md)), never `else TooWeak()`, never a failure block ([12 §12](./12-accepted-decision.md)).

## 6. Precedence cheat sheet

| Token | Role | Where |
| --- | --- | --- |
| `.` | Meet of constraints | `String.Min(3).Max(10)` |
| `or` | Join of assertions | `HasPrefix("+") or HasPrefix("0")` |
| `\|` | Join of types | typedef `"a" \| "b"`, `Success \| Failure` |
| `&` | Meet of types | typedef `A & B` |
| `else` | Typed failure of `ensure` | after a complete assertion |
| `{ … }` after `ensure` | Failure block | ordinary functions / `main`; **not** guards |
| bare `TypeName` after `is` | Type target | `ensure status is ActiveStatus` ([14](./14-type-targets.md)) |
| `Name()` after `is` | Assertion target | `ensure password is Strong()` |
| `\|\|`, `&&`, `==` | Boolean / comparison **expressions** | `if`, not `ensure`, not guard conditions |

If a user writes `or` and meant failure, the diagnostic should say so: “`or` starts another constraint; use `else` for errors.”
