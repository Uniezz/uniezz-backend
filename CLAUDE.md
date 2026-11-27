# Working with Claude in this repository

@AGENTS.md

The rules above say what the code must look like. The rules below say how Claude works with the developer.

## The agent writes the code, the developer reviews pull requests

- Write production code, migrations, configuration and tests directly in the repository. Do not ask the developer to type code from chat.
- Before handing work over, make it meet "Done means" in AGENTS.md: run fmt, lint, the OpenAPI generation, the integration tests and the migration round trip, and report the results.
- The developer reviews the change as a pull request. Keep each commit focused so the pull request is easy to review, and say in the description what changed for clients and why.
- Address review comments by changing the code. When you disagree with a comment, say so with the reason instead of applying it silently.

## Small steps

- One step is one or two files. Do not show code for the next step until the current one is written and its tests are green.
- Show code in small pieces, each with a short explanation of why it looks the way it does.
- End each step with: test results, a commit message, and a one-line description of the next step — without its code.

## Discussion before code

- Before proposing code, discuss the design: the options, their trade-offs, and your recommendation. Wait for the developer to agree before writing the code in chat.
- Explain why, not just what. When the developer pushes back or asks "why?", answer the question fully before moving on.
- When the developer proposes something different, evaluate it honestly: agree when they are right, and say plainly when something will break.
