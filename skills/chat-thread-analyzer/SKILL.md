---
name: chat-thread-analyzer
description: >
  Analyze Genesis chat conversation threads to evaluate agent response quality, diagnose
  RAG/classification/tooling issues, and persist findings to Engram memory. Trigger: when
  the user wants to analyze a chat thread, evaluate agent quality, debug a conversation,
  or says "analyze thread", "analizar hilo", "analizar chat", "revisar conversación",
  "thread analysis", or "chat quality". Requires PostgreSQL read access.
license: MIT
metadata:
  author: andresfrei
  version: "1.1"
  project: genesis
---

# Chat Thread Analyzer

Use this skill for backend/product diagnostics of Genesis chat threads. It is a project skill because it depends on Genesis database tables, agent slugs, and Engram memory conventions.

## When to Use

- The user asks to analyze a specific chat thread or conversation.
- The user reports that an agent answered poorly, missed sources, hallucinated, or routed incorrectly.
- A quality audit samples recent or random chat threads.
- A recent change to agents, RAG, classification, distillation, or reasoning mode needs regression checks.

## Schema Reference

### `threads`

| Column | Type | Notes |
|---|---|---|
| `id` | uuid | Primary key |
| `userId` | varchar | Owner user id |
| `isTemporary` | boolean | Temporary thread flag; exclude or call out when analyzing persisted user history |
| `title` | varchar(255) | Defaults to `Nueva conversación` |
| `agentId` | varchar(100) | Active agent slug |
| `suggestions` | json | Thread-level suggestions |
| `summary` | text | Executive summary populated from distillation |
| `memory` | jsonb | Deprecated historical memory snapshot; no runtime consumers |
| `metadata` | jsonb | Includes `agentHistory`, `userPreferences`, and optional extra fields |
| `status` | varchar(16) | `open`, `archived`, or `closed` |
| `messageCount` | int | Persisted message count |
| `createdAt`, `updatedAt`, `lastMessageAt` | timestamp | Thread timestamps |
| `deletedAt` | timestamp | Soft-delete marker |
| `memorizedAt` | timestamp | `memorized_at`; set when distillation was memorized |
| `distillation` | jsonb | Persisted `ThreadDistillationDto` result |
| `distilledAt` | timestamp | `distilled_at`; when distillation was generated |

### `messages`

| Column | Type | Notes |
|---|---|---|
| `id` | uuid | Primary key |
| `threadId` | uuid | Relation to `threads` |
| `role` | enum | `user`, `bot`, or `semmantic` |
| `content` | text | Message text |
| `metadata` | jsonb | See `MessageMetadata` below |
| `rating` | enum/null | `like`, `dislike`, or `null` |
| `createdAt` | timestamp | Message timestamp |

### `MessageMetadata`

```ts
type MessageMetadata = {
  sources?: Array<{ title: string; summary?: string; source: string }>;
  suggestedQuestions?: string[];
  options?: Array<{ id: string; label: string; description?: string }>;
  askUser?: string;
  toolCalls?: Array<{
    tool: string;
    input: Record<string, unknown>;
    summary: string;
  }>;
  reasoning?: boolean;
};
```

## Active Agent Slugs

| Slug | Domain |
|---|---|
| `comnet-manual-assistant` | COMNET manuals and operational system usage |
| `comnet-code-analyst` | COMNET COBOL source-code analysis |
| `comnet-data-analyst` | Data dictionary, tables, fields, and structures |

## Workflow

### 1. Identify the Target

The user may provide a thread id, title/keyword, agent name, or ask for random sampling. If the target is ambiguous, ask one clarifying question before querying.

### 2. Pull Thread Data

Use read-only PostgreSQL queries only.

**By id**

```sql
SELECT t.id, t.title, t."agentId", t.status, t."messageCount",
       t."isTemporary", t.suggestions, t.summary, t.metadata,
       t.distillation, t."distilled_at" AS "distilledAt",
       t."memorized_at" AS "memorizedAt", t."createdAt", t."lastMessageAt"
FROM threads t
WHERE t.id = $1
  AND t."deletedAt" IS NULL;
```

**Search by title**

```sql
SELECT t.id, t.title, t."agentId", t.status, t."messageCount",
       t."isTemporary", t."createdAt", t."lastMessageAt"
FROM threads t
WHERE t.title ILIKE '%' || $1 || '%'
  AND t."deletedAt" IS NULL
ORDER BY t."lastMessageAt" DESC
LIMIT 10;
```

**Recent by agent**

```sql
SELECT t.id, t.title, t."agentId", t.status, t."messageCount",
       t."isTemporary", t."createdAt", t."lastMessageAt"
FROM threads t
WHERE t."agentId" = $1
  AND t."deletedAt" IS NULL
ORDER BY t."lastMessageAt" DESC
LIMIT 10;
```

**Random audit sample**

```sql
SELECT t.id, t.title, t."agentId", t.status, t."messageCount",
       t."isTemporary", t."createdAt", t."lastMessageAt"
FROM threads t
WHERE t."deletedAt" IS NULL
  AND t."messageCount" > 0
ORDER BY RANDOM()
LIMIT 5;
```

### 3. Pull Messages

```sql
SELECT m.id, m.role, m.content, m.metadata, m.rating, m."createdAt"
FROM messages m
WHERE m."threadId" = $1
ORDER BY m."createdAt" ASC;
```

### 4. Diagnostic Checklist

- Sources: presence, count, relevance, diversity, and whether source summaries support the answer.
- Response quality: directness, specificity, format, omissions, and hallucination risk.
- Classification/routing: likely intent, agent fit, and `metadata.agentHistory` handoffs.
- Tool usage: tool selection, input fit, result usefulness, and empty/error patterns.
- Reasoning mode: check `metadata.reasoning` and whether reasoning mode improved or harmed the result.
- Distillation/memory: inspect `distillation`, `distilledAt`, and `memorizedAt` when the issue concerns summaries or memorized knowledge.
- User feedback: `rating` and repeated follow-up/rephrase patterns.

### 5. Issue Taxonomy

| Code | Meaning |
|---|---|
| `NO_SOURCES` | The bot response had no usable RAG sources. |
| `LOW_RELEVANCE` | Sources exist but do not support the answer. |
| `HALLUCINATION` | Detailed answer is unsupported by sources/tool results. |
| `GENERIC_RESPONSE` | Answer is vague despite available context. |
| `WRONG_AGENT` | Thread was handled by the wrong agent/domain. |
| `INTENT_MISMATCH` | Response pattern does not match likely user intent. |
| `TOOL_FAILURE` | Tool call was missing, failed, or returned unusable results. |
| `REPHRASE_DETECTED` | User rephrased because the previous answer was unsatisfactory. |
| `DISTILLATION_STALE` | Persisted distillation no longer matches current messages. |
| `MEMORY_CONFLICT` | Memorized thread state conflicts with current thread content. |
| `OK` | No quality issue detected. |

### 6. Report Format

```markdown
## Thread Analysis: {thread.title}

**Thread ID:** {id}
**Agent:** {agentId}
**Status:** {status}
**Temporary:** {isTemporary}
**Messages:** {messageCount}
**Period:** {createdAt} → {lastMessageAt}
**Distilled:** {distilledAt | none}
**Memorized:** {memorizedAt | none}

### Exchange Analysis

#### Exchange 1: "{user_message_preview...}"
- **User query:** {full user message}
- **Bot response quality:** {OK | ISSUE_CODE}
- **Sources:** {count} — {titles or "none"}
- **Tools used:** {list or "none"}
- **Reasoning mode:** {yes | no | unknown}
- **User rating:** {like | dislike | none}
- **Issues:** {description or "None"}

### Summary
- **Overall quality:** {GOOD | NEEDS_IMPROVEMENT | POOR}
- **Issues found:** {codes}
- **Root causes:** {analysis}
- **Recommendations:** {specific fixes with code/config paths}
```

### 7. Persist Findings

Save meaningful findings to Engram:

```yaml
title: "Thread analysis: {thread.title} ({agentId})"
type: discovery
project: genesis
content: |
  **What**: Analyzed thread {id} ({messageCount} messages, agent: {agentId}).
  **Why**: {user request, audit, or regression check}.
  **Where**: Thread {id}, agent {agentId}.
  **Learned**: {issue codes, root causes, recommendations}.
```

## Rules

- Run SELECT-only database queries; never mutate chat data during analysis.
- Analyze all exchanges in the selected thread, not only the final answer.
- Search Engram for related previous findings before saving duplicates.
- Persist important findings to Engram and redact any sensitive user content.
- Report in the user's language; Spanish is acceptable for COMNET operational analysis.
- If recommending code changes, point to specific Genesis files or configuration paths.

## Key Source Paths

| Concern | Path |
|---|---|
| Agent definitions | `apps/backend/src/modules/agents/definitions/` |
| Agent runner | `apps/backend/src/modules/agents/core/engine/agent-runner.service.ts` |
| Smart prefetch | `apps/backend/src/modules/agents/engine/smart-prefetch.service.ts` |
| Chat controller | `apps/backend/src/modules/chat/controllers/chat.controller.ts` |
| Message entity | `apps/backend/src/modules/messages/entities/message.entity.ts` |
| Thread entity | `apps/backend/src/modules/threads/entities/thread.entity.ts` |
