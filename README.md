# Forst &emsp; [![CI]][actions] [![Release]][release] [![Coverage Status]][coveralls] [![License]][license]

[CI]: https://img.shields.io/github/actions/workflow/status/forst-lang/forst/lint-test-coverage.yml
[actions]: https://github.com/forst-lang/forst/actions
[release]: https://img.shields.io/github/v/release/forst-lang/forst?filter=v*
[Coverage Status]: https://coveralls.io/repos/github/forst-lang/forst/badge.svg?branch=main
[coveralls]: https://coveralls.io/github/forst-lang/forst?branch=main
[License]: https://img.shields.io/github/license/forst-lang/forst

**A programming language that helps you move TypeScript backends to Golang.**

To accomplish this, Forst supports you in four key ways:

| Benefit | How it works |
| --- | --- |
| Go ecosystem | Compiles directly to Go. You get full access to the Go package ecosystem, standard library and build tools. |
| TypeScript integration | Generates native TS definitions and a client directly from your backend source. No need for intermediate languages like GraphQL or Protobuf just for type safety. |
| Node.js interop | Call existing Node, Bun or Deno code directly from Forst to keep your existing tools and libraries during migration. |
| Incremental migration | Allows you to move small parts of your code at a time while your existing codebase keeps running, so you can keep shipping during migration. |

## Why?

Building backend APIs often forces a trade off between runtime performance and developer speed. Maintaining separate schema contracts leads to lots of glue code. Full backend rewrites into more performant languages are often impossible to do safely.

Forst exists to remove that friction. You can migrate incrementally or write completely new backends with native performance and an ergonomic DX by design.

## Examples

### Hello World

Ordinary Forst programs look like Go. Files use the `.ft` extension.

```ft
package main

import "fmt"

func main() {
	fmt.Println("Hello World!")
}
```

More in [docs/language/overview.mdx](docs/language/overview.mdx).

### Describe a shape

Constraints on the type validate boundary data before business logic runs.

```ft
func invite(member {
	email: String.Min(5).Max(254).Contains("@"),
	role:  "admin" | "editor" | "viewer",
}) {
	// member.email contains "@" and fits common length bounds
	// member.role is one of admin, editor, viewer
	println("invite " + member.email + " as " + member.role)
}
```

More in [docs/language/shapes-and-constraints.mdx](docs/language/shapes-and-constraints.mdx).

### Check a value

Declare expected failures with `error`. Use `ensure` so a failed check returns that error and later lines can rely on the condition.

```ft
error EmptyName {}

func greet(name String) {
	ensure name is Min(1) else EmptyName{}
	return "hello, " + name
}
```

More in [docs/language/check-a-value.mdx](docs/language/check-a-value.mdx) and [docs/language/named-errors.mdx](docs/language/named-errors.mdx).

### Go interop

Import Go packages and call them from Forst. Short `ensure` forms work on bools and errors.

```ft
import "os"
import "path/filepath"

func openFile(path String) {
	ensure filepath.IsAbs(path)
	file, err := os.Open(path)
	ensure !err
	return file
}
```

More in [docs/interop/go.mdx](docs/interop/go.mdx). During migration you can also call legacy JavaScript from Forst; see [docs/interop/bridge.mdx](docs/interop/bridge.mdx).

### Call from TypeScript

Run `forst generate` so Node callers import a typed package handle and invoke over HTTP.

```typescript
import { $auth } from "@forst/gen/auth";

const result = await $auth.VerifyPassword({
  plainPassword: "secret",
  passwordHash: "$2a$...",
});
```

More in [docs/interop/invoke/call-forst.mdx](docs/interop/invoke/call-forst.mdx).

### Domain errors

Named Forst errors decode to tagged classes on the client. Use your generated package name (for example `@myapp/tictactoe`).

```typescript
import { $main } from "@myapp/tictactoe/main";
import { $CellTaken } from "@myapp/tictactoe/main/errors";

try {
  await $main.PlayMove({ state, row: 1, col: 2 });
} catch (error) {
  if (error instanceof $CellTaken) {
    console.log(error.row, error.col);
  }
}
```

In Effect mode, catch by tag:

```typescript
import { Effect } from "effect";
import { $main } from "@myapp/tictactoe/main";

const program = $main.PlayMove(req).pipe(
  Effect.catchTag("@myapp/tictactoe/CellTaken", (e) =>
    Effect.succeed({ row: e.row, col: e.col })
  )
);
```

More in [docs/interop/invoke/call-forst.mdx](docs/interop/invoke/call-forst.mdx#domain-errors).

## Features

Highlights from the [docs feature comparison](docs/why.mdx).

### Language

| Capability | Forst |
| --- | --- |
| Structural typing | [Built-in](docs/language/overview.mdx) records, signatures, and `is` narrowing |
| Validation on types | [Field constraints](docs/language/shapes-and-constraints.mdx); boundary runtime checks |
| Error handling | [`ensure`](docs/language/check-a-value.mdx), [named errors](docs/language/named-errors.mdx), and [`Result`](docs/language/result.mdx); no exceptions |
| Mocking and DI | [`use` / `with`](docs/language/providers.mdx) providers; no external DI framework |
| Type narrowing | [`is` / `ensure`](docs/language/check-a-value.mdx) and [type guards](docs/language/name-a-rule.mdx) |
| Goroutines | Native `go` and `defer` via Go output |

### Interop and adoption

| Capability | Forst |
| --- | --- |
| Go module ecosystem | [Import Go packages](docs/interop/go.mdx) natively |
| JS / npm ecosystem | Call legacy JS/TS via [`import "./path" js`](docs/interop/bridge.mdx) |
| Shared server ↔ client types | [`forst generate`](docs/interop/invoke/generate-types.mdx) from the same `.ft` source |
| Call backend from Node | [Generated client and HTTP invoke](docs/interop/invoke/call-forst.mdx) |
| Incremental migration | Mix `.ft`, `.go`, and legacy code in one codebase |

See [ROADMAP.md](./ROADMAP.md) for experimental features and planned work.

## Design Philosophy

See also [PHILOSOPHY.md](./PHILOSOPHY.md) for what guides and motivates us.

## Install and tooling

### npm packages

| Package | Purpose |
| --- | --- |
| [`@forst/cli`](./packages/cli/README.md) | Forst compiler in JS/TS projects |
| [`@forst/runtime`](./packages/runtime/README.md) | Node host for calling legacy JS/TS from Forst |
| [`@forst/sidecar`](./packages/sidecar/README.md) | Spawn or attach to `forst dev` so Node can invoke Forst over HTTP |

Install the compiler in a Node project:

```bash
npm i -D @forst/cli
npx forst version
```

`@forst/cli` pulls the matching native binary from GitHub Releases.

### VS Code extension

Optional extension in [`packages/vscode-forst`](./packages/vscode-forst).

- Registers `.ft` files in the editor
- Diagnostics via the compiler HTTP LSP (`forst lsp`)

See [`packages/vscode-forst/README.md`](./packages/vscode-forst/README.md) for installation and troubleshooting.

### Linux (.deb)

On Debian or Ubuntu, install the compiler from [GitHub Releases](https://github.com/forst-lang/forst/releases) (pick `amd64` or `arm64`):

```bash
wget https://github.com/forst-lang/forst/releases/download/vX.Y.Z/forst_X.Y.Z-1_amd64.deb
sudo apt install ./forst_X.Y.Z-1_amd64.deb
forst version
```

See [docs/installation.mdx](docs/installation.mdx) for other install paths (npm, native binary, Docker).

## TypeScript client output

`forst generate` writes a typed client under `.forst/client` and links it as `@forst/gen` (or the `packageName` you set in `ftconfig.json`). Front ends and Node callers import the same shapes your server uses, without copying types by hand.

See [docs/interop/invoke/generate-types.mdx](docs/interop/invoke/generate-types.mdx) and [docs/installation.mdx](docs/installation.mdx#generated-typescript-client).

## Inspirations

Our primary inspiration is TypeScript's structural type system and its enormous success in making JavaScript development more ergonomic, robust and gradually typeable. We aim to bring similar benefits to Go development, insofar as they are not already present.

We also draw inspiration from:

- **Zod** — constraints and shape guards as composable runtime checks on nested data.
- **tRPC** — one source of truth for API shapes, with **TypeScript types and a small client** generated from Forst (`forst generate`, `examples/client-integration/`).
- **Go** and **Rust** — **errors as values** and explicit control flow (`ensure` … `else …`) instead of exceptions.