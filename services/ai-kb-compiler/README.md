# services/ai-kb-compiler — AI KB Compiler

> **Stub** — built in Build Prompt 4 (AI Cascade).

Python. Batch job (cron or on-demand). Two entry points:
- Paste ingestion: raw text/markdown → LLM drafts OKF concept files
- Dormant conversation mining: clusters unanswered questions → drafts pattern/concept candidates → writes `automation_suggestions` rows
