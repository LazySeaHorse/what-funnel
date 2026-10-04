# services/ai-answer-svc

Python. Consumes `conversation.updated` from Redis Streams, debounces inbound bubbles per conversation
(first bubble 4 s, second 4 s, third and later 6 s: `AI_DEBOUNCE_*_SECONDS`) and runs the reply cascade once
per debounced batch (1-3 bubbles of ONE conversation, never across customers).

## Cascade

1. Non-text bubble: human handoff (unchanged).
2. Local pre-checks, no model call (`router.local_precheck`, `local_checks.py`):
   - Safety backstop: legal THREATS ("my lawyer will...", "sue you", "legal action") and acute emergencies (cannot breathe,
     chest pain, overdose, gas leak, carbon monoxide, fire...), with word boundaries. A hit is a handoff.
   - First-message greeting: if nothing was ever sent in the conversation and every bubble is a greeting or common
     starter, reply with the account's `ai_greeting_text` (default "Hi! Thanks for reaching out. How can we help you today?").
   - Context-less fragment: a message that opens with a continuing word ("and on saturdays?", "what about sunday") in a
     conversation with no earlier turn is handed off instead of guessed.
   All customer text is NFKC-normalised and stripped of zero-width/bidi/control characters first (`untrusted.py`).
3. Router (`router.py`, `PROMPT_VERSION=router-v2`): ONE schema-constrained LLM call. Static prefix first (instructions, the
   account's approved FAQ menu, at most 10 FAQs, oldest first; the API/panel shows which FAQs are excluded), variable part
   last (recent turns, then the delimited, untrusted customer messages). Output is enums only: `route`
   (faq | kb | handoff | ignore), `faq_id`, `faq_coverage` (full | partial | none), `handoff_reason`
   (none | spam | needs_human | prompt_injection | other). No tools, no free text reaches the customer.
4. Deterministic gates (`router.apply_gates`):
   - canned FAQ: sent verbatim only if `faq_id` is a menu id, coverage is `full` and there is no handoff reason; partial or
     no coverage falls to the grounded-answer stage.
   - `ignore` is honoured only if the whole message is a bare acknowledgement (`local_checks.is_acknowledgement`); otherwise it
     becomes a low-priority soft review flag.
   - `prompt_injection` flags that single message (soft review flag): nothing is sent, the chat stays active and later messages
     are gated independently. The flag lives in `conversation_ai_state.review_flag_*` and is cleared by a human message.
   - `needs_human` / backstop: escalation (below). `spam`: no reply.
5. Grounded answer stage (`kb_rag.py`, `grounding.py`): sources are ALL approved FAQ answers plus retrieved `kb_concepts`
   (embeddings only retrieve concepts; contextual query, relevance floor `AI_KB_MIN_SIMILARITY`). The answer must cite valid
   source numbers and every number (whole-number match with units, spelled-out numbers converted), e-mail/URL and capitalised
   name must be present in the cited sources. Facts without a number, locator or name are NOT checked, so generated answers
   are draft-only unless `AI_RAG_AUTO_SEND` or the account setting `ai_rag_auto_send` is true. Canned and greeting replies
   follow the normal reply-mode rules.
6. Failure anywhere (provider error, timeout, invalid structured output, ungrounded answer) fails closed to a handoff.

Handoff behaviour: escalations (needs_human, backstop) end AI replies on the chat (`review_required`) and, in auto-send mode
only, send one plain acknowledgement, `control.HANDOFF_ACK_REPLY` ("Your message has been transferred to a human agent.").
Spam gets no reply in any mode. Unanswerable messages use the existing cooldown path with the same acknowledgement in auto-send
mode. Soft flags (injection, rejected ignore) send nothing and change no state. In draft-only mode no customer message is sent.
Reply modes (workspace default, per-user override, per-chat override) are resolved exactly as before.

Every router decision is logged to `ai_router_decisions` (route, FAQ, handoff reason, outcome, latency, token usage,
prompt version, error). Answer events and drafts use the stages `greeting | canned | rag | handoff | ignored | none`.

## Conversation summaries

`summary.py` generates the per-conversation summary shown in the inbox ("Summarize conversation"). One function,
`generate_summary(..., trigger)`, serves two Redis streams (consumer group `ai-answer-svc-group`):

| stream | trigger | policy |
|---|---|---|
| `conversation.closed` | `closed` | debounced: first summary needs a message; later ones need `SUMMARY_MIN_INTERVAL_SECONDS` since the last and new messages |
| `conversation.summary_requested` (published by conversation-svc `POST /conversations/{id}/summary`) | `requested` | bypasses the 60 s rule: unchanged conversation returns the cached row, new messages regenerate |

Both take a per-conversation Redis lock (`summary:lock:<id>`, released by token) so concurrent runs never duplicate an LLM call;
requests also respect a short cooldown (`summary:cooldown:<id>`, `SUMMARY_REQUEST_COOLDOWN_SECONDS`) set after every
generated or failed attempt. The last `SUMMARY_MAX_MESSAGES` text messages are sanitised with `untrusted.sanitize_untrusted`,
flattened to one line each and passed between `<<<CONVERSATION START/END>>>` markers. Fields come from the account setting
`summary_schema` (a list of `{key,label,description}` or a `{key: description}` map; invalid keys are dropped, default is
customer_wants / preferred_timeframe / objections / next_action) and are validated: exactly the schema keys, plain text,
capped at `SUMMARY_FIELD_MAX_CHARS`, `N/A` when missing. One row per conversation in `conversation_summaries`
(`message_count_at_generation` marks staleness). Nothing is written on failure.

Events published: `conversation.summary_updated` (`account_id`, `conversation_id`, `summary_fields`, `generated_at`,
`message_count_at_generation`) and, for on-demand requests only, `conversation.summary_failed` (`error_code`:
`ai_not_configured | provider_error | no_messages | rate_limited | internal_error`, plus a client-safe `message`).

## Evaluation harness

`eval/` holds three labelled slices and `eval/harness.py`, which runs the real path (pre-checks, prompt, schema, gates and
the grounded-answer stage) against any OpenAI-compatible endpoint:

| slice | file | menus | purpose |
|---|---|---|---|
| seen | `cases.jsonl` (196) | `menus.json` (dental, bakery, bikes) | original cases with the checker's corrected labels; the `not_for` notes were written next to these cases, so near-miss scores are an upper bound |
| heldout | `cases_heldout.jsonl` (60) | `menus_heldout.json` (salon, gym, plumber) | unseen menus with generic notes, plus new phrasings: the better predictor for a new tenant |
| kb | `cases_kb.jsonl` (40) | both, plus `kb.json` concepts | answerable from a concept, from FAQ answers, from both, and unanswerable (hallucination bait) |

```bash
cd services/ai-answer-svc
export EVAL_BASE_URL=https://generativelanguage.googleapis.com/v1beta/openai   # any OpenAI-compatible /v1 base
export EVAL_API_KEY=...
export EVAL_MODEL=gemini-3.1-flash-lite            # default
export EVAL_PRICE_IN=0.10 EVAL_PRICE_OUT=0.40      # optional, USD per 1M tokens, enables the cost estimate
PYTHONPATH=.:../../packages/python python -m eval.harness --min-interval 4.5 --cache /tmp/eval-cache.jsonl --json /tmp/eval.json
# or from the repo root: make eval-router EVAL_ARGS="--only-slice heldout"
```

It reports, overall and per slice: lenient and strict route accuracy, precision/recall per route, canned-reply precision and
recall, false-canned rate, wrong FAQ ids, escalation and injection recall, silent drops (messages ignored without a flag),
KB answer correctness, hallucinations (forbidden content in a generated answer) and answers blocked by the grounding gate,
a confusion matrix, per-kind accuracy and latency/tokens/cost, and lists every miss. The KB stage gets the menu's FAQ answers
plus ALL concepts of the menu (no embedding retrieval in the harness). `--cache` replays identical requests and only caches
successes, so re-running after a 429 repeats just the failed calls; `--min-interval` paces live calls; 429/5xx are retried
with Retry-After and backoff. `--only-slice`, `--only-kind`, `--only-ids`, `--limit` select subsets.
Unit tests use fake clients (`tests/test_eval_harness.py`).

## Tests

```bash
PYTHONPATH=services/ai-answer-svc:packages/python pytest services/ai-answer-svc/tests
```
Redis- and Postgres-backed tests skip when `REDIS_URL` / `DATABASE_URL` are not reachable.

## Configuration

`AI_REQUEST_TIMEOUT_SECONDS` (20), `AI_PROVIDER_MAX_ATTEMPTS` (2), `AI_CASCADE_DEADLINE_SECONDS` (45, hard cap on router + KB, fails closed to a handoff), `AI_RUN_RECLAIM_SECONDS` (deadline + 60),
`AI_DEBOUNCE_FIRST_SECONDS` (4), `AI_DEBOUNCE_SUBSEQUENT_SECONDS` (4), `AI_DEBOUNCE_BURST_SECONDS` (6),
`AI_ROUTER_MAX_TOKENS` (150), `AI_KB_MAX_TOKENS` (400), `AI_KB_MIN_SIMILARITY` (0.30, uncalibrated),
`AI_CASCADE_CONCURRENCY` (8), `AI_DEBOUNCE_MAX_ATTEMPTS` (3),
`SUMMARY_MIN_INTERVAL_SECONDS` (60), `SUMMARY_REQUEST_COOLDOWN_SECONDS` (10), `SUMMARY_MAX_MESSAGES` (50), `SUMMARY_FIELD_MAX_CHARS` (600).
