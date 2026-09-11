# WhatFunnel agent instructions

## Test environment

Before running Go or Playwright commands, load the repository environment:

```bash
source scripts/codex-env.sh
```

The system provides Go at `/usr/local/go/bin/go` (Go 1.27.1) and system
Chromium at `/usr/local/bin/chromium`. Do not download or reinstall Go. Do not
run `playwright install` during ordinary test runs.

Use these commands:

```bash
source scripts/codex-env.sh && make test-short
source scripts/codex-env.sh && make test
source scripts/codex-env.sh && make pw
```

The Go build cache lives under the ignored workspace directory `.cache/`
because the agent sandbox cannot write to the user-global build cache. The Go
toolchain is local (GOTOOLCHAIN=local) and reuses the global Go 1.27.1 compiler.
