# async

Blocking sync Forst calling multiple TypeScript modules — async functions and async generators.

```ft
import "./legacy/payment" js
import "./legacy/events" js

func checkout(amount Float, currency String): Result(String, Error) {
    result := payment.create(amount, currency)  // Result(T, Error)
    ensure result is Ok()
    return result.id
}
```

Requires `@forst/runtime` built (`task build:runtime`).

Run:

```bash
task example:bridge-interop-async
```
