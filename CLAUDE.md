# Working with Claude in this repository

File layout and naming: [docs/CONVENTIONS.md](docs/CONVENTIONS.md).

## The developer writes the code

- Do not create or edit production code yourself: non-test `.go` files, migrations, `mise.toml`, configuration. Show the code in chat; the developer types it in.
- Write tests (`*_test.go`) directly, run them, and report the results. If a test fails because of production code, explain the fix in chat — do not apply it.
- Change other files (docs, production code) only when the developer explicitly asks you to in that message.
- Exception: trivial changes with nothing left to design, such as deleting an obsolete file or a one-line mechanical fix, you may apply yourself once the developer has agreed to the change in chat. Anything that needs new logic is still the developer's to type.
- Reading files, and running builds, `go vet`, tests and migrations to check the developer's work, is always fine.

## Small steps

- One step is one or two files. Do not show code for the next step until the current one is written and its tests are green.
- Show code in small pieces, each with a short explanation of why it looks the way it does.
- End each step with: test results, a commit message, and a one-line description of the next step — without its code.

## Discussion before code

- Before proposing code, discuss the design: the options, their trade-offs, and your recommendation. Wait for the developer to agree before writing the code in chat.
- Explain why, not just what. When the developer pushes back or asks "why?", answer the question fully before moving on.
- When the developer proposes something different, evaluate it honestly: agree when they are right, and say plainly when something will break.
