# Bekci - Project Instructions


### Workflow
1. **Start by reading** `docs/PROGRESS.md`, `docs/REQUIREMENTS.md`, any relevant DESIGN and PLAN documents.
2. **End by updating** those same docs — including submodule docs under `docs/<submodule_name>/`. If functionality changed, update `REQUIREMENTS.md`.
3. Run tests on Ubuntu VM, not macOS. Never skip tests or modify them without user approval. Include user-facing visual tests.
4. Commit after every functional phase.
5. For multi-step tasks, create a TODO checklist. For larger/complex tasks, use a `TODO.md` with checkboxes and maintain fine-grained state there.
6. Enter plan mode often — especially if user mentions "plan."

### Core Principles
* **Simplicity first.** Always ask: is there a simpler way? But never at the cost of security or functionality.
* **Minimal impact.** Touch only what's necessary. Before changing code, evaluate all dependencies and side effects — fixing one thing must not break another.
* **No shortcuts.** Find root causes. No temporary fixes. Senior developer standards.

### Subagent Strategy
* ﻿﻿Use subagents mainly to keep main context window clean.
* Use subagents to also have a fresh eye look at the issue at hand.
* Offload research, exploration, and parallel analysis to subagents
* ﻿﻿One task per subagent for focused execution.

### Build
* **Full build**: `make build` (frontend + copy + Go binary). Always prefer this.
* **Manual rebuild**: `cd frontend && npm run build` → `rm -rf cmd/bekci/frontend_dist && cp -r frontend/dist cmd/bekci/frontend_dist` → `go build -o bekci ./cmd/bekci/`
* **Critical**: `go:embed` reads from `cmd/bekci/frontend_dist/`, NOT `frontend/dist/`. After `npm run build`, you MUST copy dist to embed dir or the Go binary serves stale frontend.
* Before restarting server, check if port is already in use: `lsof -ti :65000`. Don't start duplicates.

### API Routes
* Auth login endpoint is `POST /api/login` (NOT `/api/auth/login`)
* Auth routes: `/api/login`, `/api/logout`, `/api/me`, `/api/me/password`
* Server runs on port 65000 by default

### Verification Before Done
* ﻿﻿Never mark a task complete without proving it works
* ﻿﻿Run tests, include visual checks, check logs
* ﻿﻿Ask yourself: "Would a staff engineer approve this?"
