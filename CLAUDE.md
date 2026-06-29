# go-search — Learning Project

## Goal
Build an Elasticsearch-inspired, in-process full-text search engine SDK in Go.
The engine runs entirely inside the caller's Go process — no external service needed.

## Architecture (build in this order)
```
analysis/    → tokenizer + filters + analyzer pipeline
index/       → inverted index (token → posting list)
scoring/     → BM25 relevance ranking
query/       → boolean query (must / should / must_not)
engine/      → public SDK surface (Index, Search, Delete)
```

## Teaching Workflow
1. Claude creates a task file in `task/` before each new task
2. The task file includes: what to implement, the concept behind it, and references
3. The user writes the code
4. Claude reviews, explains issues, and moves to the next task

## Task Files
Each task lives in `task/XX-name.md`. Format (in this order):
1. **Why** — the problem this solves and why it matters in a real search engine
2. **What to implement** — the interface/signatures to build
3. **Concept** — how it works, the algorithm, key design decisions
4. **Acceptance criteria** — concrete pass/fail conditions
5. **References** — links for deeper reading

## Rules for Claude
- Do NOT write implementation code for the user — explain and review only
- Do write test files when the user asks (tests illustrate expected behavior)
- Point out bugs by explaining what's wrong, not by silently fixing them
- Keep explanations short; link to references for deeper reading
