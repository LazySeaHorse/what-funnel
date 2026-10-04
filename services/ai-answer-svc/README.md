# services/ai-answer-svc

Python. Consumes `conversation.updated` from Redis Streams, debounces inbound bubbles per conversation
(first bubble 4 s, second 4 s, third and later 6 s: `AI_DEBOUNCE_*_SECONDS`) and runs the reply cascade once
per debounced batch (1-3 bubbles of ONE conversation, never across customers).

## Cascade

1. Non-text bubble: human handoff (unchanged).
2. Local pre-checks, no model call (`router.local_precheck`):
   - Safety backstop: legal threats and acute medical emergencies only (`router.backstop_hit`). A hit is a handoff.
   - First-message greeting: if nothing was ever sent in the conversation and every bubble is a greeting or common
     starter ("hi", "good morning", "anyone there?", "i have a question", with trivial punctuation/emoji), reply with the
     account's `ai_greeting_text` (account setting, default "Hi! Thanks for reaching out. How can we help you today?").
     Deterministic and anchored on the whole message, so it needs no model call and cannot be steered by customer text.
3. Router (`router.py`): ONE schema-constrained LLM call. Static prefix first (instructions, output schema description,
   the account's approved FAQ menu, at most 10 FAQs, oldest first, extras excluded with a logged warning), variable part
   last (recent turns, then the delimited, untrusted customer messages). Output is enums only:
   `route` (faq | kb | handoff | ignore), `faq_id` (F1.. | none), `faq_covers_everything`, `handoff_reason`
   (none | spam | needs_human | prompt_injection | other). No tools, no free text reaches the customer.
4. Deterministic gates (`router.apply_gates`): a canned FAQ is sent verbatim only if `faq_id` is a menu id, the FAQ fully
   covers the message and there is no handoff reason. Otherwise it falls to the KB stage or a handoff.
5. KB stage (`kb_rag.py`): embeddings are used only to retrieve `kb_concepts`, with a contextual query (greetings dropped,
   previous customer turn prepended for short follow-ups) and a relevance floor (`AI_KB_MIN_SIMILARITY`). The answer is
   sent only if it cites a retrieved concept and every number, time, price, URL and e-mail in it appears in a cited concept.
6. Failure anywhere (provider error, timeout, invalid structured output, ungrounded answer) fails closed to a handoff.

Handoff behaviour: escalations (needs_human, prompt injection, backstop) end AI replies on the chat (`review_required`) and,
in auto-send mode only, send one plain acknowledgement, `control.HANDOFF_ACK_REPLY`
("Your message has been transferred to a human agent."). Spam gets no reply in any mode. Unanswerable messages use the
existing cooldown path with the same acknowledgement in auto-send mode. In draft-only mode no customer message is sent.
Reply modes (workspace default, per-user override, per-chat override) are resolved exactly as before.

Every router decision is logged to `ai_router_decisions` (route, FAQ, handoff reason, outcome, latency, token usage,
prompt version, error). Answer events and drafts use the stages `greeting | canned | rag | handoff | ignored | none`.

## Evaluation harness

`eval/` holds a labelled set (`cases.jsonl`, 3 fictional businesses x 5 FAQs in `menus.json`) and `eval/harness.py`,
which runs the real routing path (pre-checks, prompt, schema, gates) against any OpenAI-compatible endpoint:

```bash
cd services/ai-answer-svc
export EVAL_BASE_URL=https://generativelanguage.googleapis.com/v1beta/openai   # any OpenAI-compatible /v1 base
export EVAL_API_KEY=...
export EVAL_MODEL=gemini-3.1-flash-lite            # default
export EVAL_PRICE_IN=0.10 EVAL_PRICE_OUT=0.40      # optional, USD per 1M tokens, enables the cost estimate
PYTHONPATH=.:../../packages/python python -m eval.harness --json /tmp/eval.json --cache /tmp/eval-cache.jsonl
# or from the repo root: make eval-router EVAL_ARGS="--limit 20"
```

It reports precision/recall per route, canned-reply precision/recall, false-canned rate, handoff (escalation) recall and
reason accuracy, a confusion matrix, per-kind accuracy and latency/tokens/cost, and lists every false canned reply and missed
escalation. `--cache` replays identical requests from a JSONL file to save quota; `--limit`, `--only-kind`, `--concurrency`
and `--min-interval` help with rate-limited keys. Unit tests use a fake client (`tests/test_eval_harness.py`).

## Tests

```bash
PYTHONPATH=services/ai-answer-svc:packages/python pytest services/ai-answer-svc/tests
```
Redis- and Postgres-backed tests skip when `REDIS_URL` / `DATABASE_URL` are not reachable.

## Configuration

`AI_REQUEST_TIMEOUT_SECONDS` (20), `AI_PROVIDER_MAX_ATTEMPTS` (2), `AI_CASCADE_DEADLINE_SECONDS` (45, hard cap on router + KB, fails closed to a handoff), `AI_RUN_RECLAIM_SECONDS` (deadline + 60),
`AI_DEBOUNCE_FIRST_SECONDS` (4), `AI_DEBOUNCE_SUBSEQUENT_SECONDS` (4), `AI_DEBOUNCE_BURST_SECONDS` (6),
`AI_ROUTER_MAX_TOKENS` (150), `AI_KB_MAX_TOKENS` (400), `AI_KB_MIN_SIMILARITY` (0.30, uncalibrated),
`AI_CASCADE_CONCURRENCY` (8), `AI_DEBOUNCE_MAX_ATTEMPTS` (3).
