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

### Verification Before Done
* ﻿﻿Never mark a task complete without proving it works
* ﻿﻿Run tests, include visual checks, check logs
* ﻿﻿Ask yourself: "Would a staff engineer approve this?"
