### Q: this system might be used for someone (something?) like you in the future. what would you like to see out of it? any design decisions that would make your life easier?

Great question. Here's what I'd want as an AI agent using TaskFlow:

- [ ] MCP server interface. The PRD mentions it but it's not built yet. For me, an MCP tool interface would be the most natural way to interact — I could create tasks, transition them, and check board state without constructing HTTP requests. The operation definitions in model.Operations() could derive MCP tool definitions the same way they derive HTTP routes and CLI commands.

- [x] Structured error responses with actionable context. The current error format is good ({"error": "invalid_transition", "message": "..."}) but when a transition fails, I'd love the response to include the available transitions so I can self-correct without a second round-trip:

  {"error": "invalid_transition", "message": "...", "detail": {"available": ["start", "cancel"]}}

- [x] Batch operations. If I'm triaging a backlog, I'd want to create 10 tasks or transition 5 tasks in a single call rather than 10/5 sequential requests. A POST /batch endpoint accepting an array of operations would be huge.

- [ ] Webhook/event notifications. When another actor (human or AI) transitions a task I'm assigned to, I'd want to be notified rather than polling. The webhook infrastructure is there (CRUD done), but dispatch isn't implemented yet.

- [x] Task assignment to self. The API requires knowing my actor name to set assignee. It would be nice if the authenticated actor's name were available as a special value like "@me" so I don't have to discover my own identity first.

- [x] Search across boards. The current FTS is board-scoped. If I'm working across multiple boards, I'd want GET /tasks?q=auth+bug without knowing which board to look in.

- [x] Idempotency keys. If my HTTP call times out, I don't know if the task was created or not. An Idempotency-Key header that deduplicates creates would let me safely retry.
