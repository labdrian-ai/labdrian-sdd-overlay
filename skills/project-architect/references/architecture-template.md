# project-architect — Architecture Document Template

Use this format exactly. Every section is mandatory except where noted.

```markdown
# Arquitectura Técnica — {Project Name}

**Fecha**: {current date}
**Basado en**:
- Manifest contexto: `project/{project}/manifest/context`
- Manifest reglas: `project/{project}/manifest/rules`
- Manifest misión (si aplica): `project/{project}/manifest/mission`
- Stack técnico: `sdd-init/{project}`
- Versión previa (si aplica): `project/{project}/architect/final`

---

## 1. Estilo Arquitectónico

**Elección**: {Monolito modular | Hexagonal | Clean Architecture | Layered | Event-Driven | etc.}

**Justificación**:
{Why this style, tied explicitly to manifest constraints and preferences. Reference specific manifest sections. Do NOT default to "industry standard" — every style choice must be defended against THIS manifest.}

**Respeta la preferencia del manifest**: {Yes/No + explanation}. Si la preferencia del manifest era otra, explicar por qué esta es mejor con evidencia técnica (no estética).

---

## 2. Módulos y Contratos Técnicos

{For each conceptual module in the manifest, provide a technical contract:}

### 2.1 Módulo: {Name}

**Propósito**: {One sentence}
**Responsabilidad única**: {What it owns, what it does NOT own}
**Inputs**: {Data/events that enter this module — types and sources}
**Outputs**: {Data/events that leave this module — types and destinations}
**Invariantes**: {Conditions this module guarantees at all times}
**Decisiones autorizadas**: {What this module is allowed to decide}
**Decisiones prohibidas**: {What this module is NOT allowed to decide}
**Dependencias permitidas**: {Modules or libraries it may depend on}
**Dependencias prohibidas**: {Modules or libraries it must NOT depend on — and why}
**Persistencia**: {Where this module stores state, if any}
**Riesgos técnicos**: {Specific risks tied to this module's design}

{Repeat for every conceptual module identified in the manifest. If a conceptual module is too vague to produce a technical contract, mark it as `[PENDIENTE DE FORMALIZACIÓN]` and explain what additional info is needed.}

---

## 3. Componentes Clave (detalle técnico)

{For modules that contain multiple components, break them down:}

### 3.1 Componente: {Name}
**Módulo padre**: {Module it belongs to}
**Responsabilidad**: {What it does}
**Interfaz pública**: {Methods/endpoints exposed}
**Dependencias**: {What it depends on}
**Límites**: {What it does NOT do}

{Only include components that are non-trivial or that have specific boundaries worth documenting. Do not inflate this section with obvious components.}

---

## 4. Modelo de Datos (alto nivel)

**Enfoque**: {SQL relacional | NoSQL documental | Event Sourcing | CQRS | híbrido}
**Justificación**: {Tied to manifest and stack}

### 4.1 Entidades principales

{For each main entity:}

**{Entity Name}**
- Atributos clave: {list}
- Relaciones: {with which other entities and how}
- Invariantes: {constraints that must always hold}
- Lifecycle: {when created, when archived/deleted, who can mutate}

### 4.2 Estrategia de persistencia

{How data is actually stored: single DB, multiple DBs, caching layers, read models, etc.}

### 4.3 Consistencia

{Strong/eventual, transactional boundaries, conflict resolution}

---

## 5. Integraciones Externas

{For each external system the project must integrate with:}

### 5.1 Integración: {Name}
**Qué es**: {External system}
**Por qué lo necesitamos**: {Reason tied to manifest}
**Cómo integramos**: {REST / gRPC / WebSocket / Webhook / SDK / Database / Message Queue}
**Frecuencia**: {Real-time / batch / on-demand}
**Manejo de errores**: {Retry, circuit breaker, fallback, etc.}
**Acoplamiento**: {How we isolate the rest of the system from this integration's instability}

{If the project has no external integrations in this phase, state "No hay integraciones externas en esta etapa" and explain why.}

---

## 6. Autenticación y Autorización

**Mecanismo de autenticación**: {JWT / OAuth / Session / API Keys / None}
**Justificación**: {Why this mechanism for this project}

**Modelo de autorización**: {RBAC / ABAC / ACL / None}
**Roles/scopes**: {If applicable, list them}

**Flujos de auth**:
{Describe the main flows: login, logout, token refresh, permission check}

**Límites**:
{What this auth model does NOT cover and when we'd need to change it}

{If the project has no auth needs in this phase, state so explicitly.}

---

## 7. Seguridad

**Amenazas consideradas**: {List the main threats relevant to this project}
**Mitigaciones implementadas**: {For each threat, what defensive measure}
**Datos sensibles**: {What's sensitive, how it's protected, where it lives}
**Auditabilidad**: {What gets logged for security auditing}
**Compliance si aplica**: {PCI, HIPAA, GDPR, etc. — only if the manifest or domain requires it}

---

## 8. Escalabilidad y Performance

**Carga esperada (inicial)**: {Numbers: requests/sec, concurrent users, data volume}
**Carga esperada (12 meses)**: {Projection}
**Estrategia de escalado**: {Vertical / horizontal / hybrid + when each kicks in}
**Puntos de contención conocidos**: {What will bottleneck first and how we handle it}
**SLOs objetivo**: {Latency, availability, error rate}

{Be honest — if the project is small and scale doesn't matter yet, say so. Don't inflate this section with premature optimization.}

---

## 9. Observabilidad

**Logging**: {Structure, levels, destinations}
**Métricas**: {What we measure, where they're exposed}
**Tracing**: {If applicable, how distributed tracing works}
**Health checks**: {Liveness, readiness, custom health}
**Alertas**: {What triggers alerts, who receives them}
**Auditabilidad (ligada al manifest)**: {Specifically address manifest Rule 5 — "Trazabilidad obligatoria". How is every decision reconstructible?}

---

## 10. Despliegue y Operaciones

**Entornos**: {dev / staging / prod — or simpler if the manifest constrains it}
**CI/CD**: {Pipeline, testing gates, deployment strategy}
**Configuración**: {How config is managed across environments — env vars, secret stores, etc.}
**Rollback**: {How we roll back a bad deployment}
**Backup y recovery**: {If applicable}

---

## 11. Riesgos Técnicos

{NOT strategic risks — those live in manifest Doc 1. Here we document technical risks specific to this architecture:}

| Riesgo | Probabilidad | Impacto | Mitigación |
|--------|--------------|---------|------------|
| {Risk 1} | {Alta/Media/Baja} | {Alto/Medio/Bajo} | {How we handle it} |
| {Risk 2} | ... | ... | ... |

---

## 12. Trade-offs Explícitos

{For each major architectural decision, document what was given up:}

### 12.1 Trade-off: {Decision}
**Elegimos**: {What}
**Sacrificamos**: {What we gave up}
**Por qué fue aceptable**: {Justification tied to manifest constraints}

{Minimum 3 trade-offs for any non-trivial architecture. If you can't find 3, you probably haven't thought hard enough.}

---

## 13. Alternativas Descartadas

{For major decisions (style, persistence, auth, integrations), 1-2 alternatives considered and rejected:}

### 13.1 Alternativa: {Name}
**Qué era**: {Description}
**Por qué la consideramos**: {Reason}
**Por qué la descartamos**: {Concrete reason tied to manifest or stack, NOT "personal preference"}

---

## 14. Evolución Futura

**Próximos 3 meses**: {What stays the same, what might change}
**6-12 meses**: {Projected evolution}
**Puntos de inflexión**: {When we'd need to revisit this architecture from scratch}
**Lo que esta arquitectura NO pretende soportar**: {Be explicit about what future needs would break this design}

---

## 15. Cumplimiento de Reglas del Manifest

{This section is MANDATORY. For each mandatory rule from `manifest_rules`, document explicitly how the architecture respects it:}

| Regla del manifest | Cómo la respeta la arquitectura |
|--------------------|--------------------------------|
| {Rule 1} | {Specific architectural decision or pattern that enforces this rule} |
| {Rule 2} | ... |

{If any rule is NOT respected by the architecture, you have two options:
(a) Redesign the architecture to respect it (preferred)
(b) Document the tension explicitly as a trade-off and propose mitigation
You may NOT silently skip a rule.}

---

## Estado
**Versión**: {1 if first, n+1 if refinement}
**Listo para**: Implementation phase (SDD pipeline o trabajo manual)
**Próximos pasos recomendados**:
- Si trabajás con SDD: correr `/sdd-init` (si no está) y después `/sdd-new` para el primer change
- Si trabajás manual: esta arquitectura sirve como guía vinculante para el equipo
- Re-ejecutar `/project-architect` si el manifest cambia significativamente
```

---

## Structured Response Envelope

Return EXACTLY this format to the orchestrator:

```markdown
## Project Architect — {status}

**Project**: {project name}
**Status**: {blocked | proceed | updated}
**Artifact store mode**: {engram | openspec | hybrid | none}
**Artifacts**: {list of engram topic keys and/or file paths persisted, or "none" if blocked}

### Executive Summary
{2-3 sentence summary in Spanish of what was produced. If blocked, summarize why.}

### Key Decisions
{If proceed: 3-5 bullet points with the most important architectural decisions. If blocked: "Ninguna — bloqueado por falta de inputs."}

### Trade-offs Summary
{If proceed: list the top 3 trade-offs explicitly. If blocked: omit.}

### Next Step
{What happens next:
- blocked: "Usuario debe elegir Opción 1, 2 o 3 (ver Step 2a)."
- proceed: "Arquitectura propuesta lista. Si el proyecto sigue con SDD, correr /sdd-init (si falta) y después /sdd-new para el primer change."
- updated: "Arquitectura actualizada. Versión previa sobrescrita en engram."}

### Risks
- {Top technical risk 1}
- {Top technical risk 2}

### Warnings
{If sdd-init context was not found, warn here.}
{If manifest was partial, warn about which sections were missing.}
{If architecture tensions exist with manifest rules, warn here.}
```
