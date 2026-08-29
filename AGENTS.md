# WhatFunnel agent instructions

## Test environment

Before running Go or Playwright commands, load the repository environment:

```bash
source scripts/codex-env.sh
```

The Codex image already provides Go at `/usr/local/go/bin/go` and stores
Playwright browsers in the user cache. Do not download or reinstall Go. Do not
run `playwright install` during ordinary test runs.

Use these commands:

```bash
source scripts/codex-env.sh && make test-short
source scripts/codex-env.sh && make test
source scripts/codex-env.sh && make pw
```

If dependencies or the required Chromium revision are genuinely absent, run
`scripts/codex-setup.sh` once. Configure that script as the Codex environment's
setup script so its result is included in the cached container.

The Go build cache lives under the ignored workspace directory `.cache/`
because the agent sandbox cannot write to the user-global build cache. The Go
module and toolchain cache remains user-global so the preinstalled Go 1.25.0
toolchain can be reused.
