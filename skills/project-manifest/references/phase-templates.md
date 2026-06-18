# project-manifest — Phase Output Templates

## Discovery Phase Output

```markdown
# Manifiesto — Descubrimiento: {Project Title}

## Mi Entendimiento
{Reformulation of the idea — 2-3 sentences max}

## Lo que ya sabemos (desde sdd-init)
{Stack, testing, conventions detected by sdd-init — 2-4 bullet points. Omit this section if no sdd-init context was found.}

## Supuestos Visibles
- {Assumption 1 — clearly marked as assumption}
- {Assumption 2}

## Brechas Críticas Detectadas
- {Gap 1 — what critical information is missing and which doc it affects}
- {Gap 2}

## Preguntas Prioritarias
{3-6 numbered questions ordered by importance. Each question should be concrete, actionable, and tied to a specific manifest document.}

1. **[Doc 1 - Naturaleza]** ¿{Specific question}?
2. **[Doc 1 - Problema central]** ¿{Specific question}?
3. **[Doc 2 - Éxito]** ¿{Specific question}?
4. **[Doc 3 - Restricciones]** ¿{Specific question}?

## Proceso
Primero levantamos el contexto estratégico del proyecto (Doc 1), después definimos la misión del agente y los criterios de éxito (Doc 2), y finalmente las reglas obligatorias de trabajo (Doc 3). Cuando los tres estén consolidados, actualizamos el skill registry para que todo el pipeline SDD los respete automáticamente.

## Estado
**Fase**: `discovery`. Se necesitan las respuestas anteriores para avanzar a `draft`.
```

---

## Draft Phase Output

```markdown
# Manifiesto — Borrador: {Project Title}

## Documento 1 — Contexto Estratégico

### 1. Naturaleza del proyecto
{What the project is and what it is NOT in this stage. Use markers.}

### 2. Universo inicial
{Initial scope: venues, assets, actors, boundaries. Use markers.}

### 3. Núcleo conceptual
{The main pillars/ideas the project stands on. Use markers.}

### 4. Estado actual del conocimiento
{What is defined, what is missing. Use markers.}

### 5. Supuestos operativos iniciales
{Marked as [HIPÓTESIS] — NOT truths.}

### 6. Problema central
{The real uncertainty driving the project. Use markers.}

### 7. Restricciones de diseño acordadas
{Hard constraints. Use markers.}

### 8. Preferencia arquitectónica (macro)
{Monolith modular / distributed / etc. — ONLY the high-level choice, NOT the full architecture. Use markers.}

### 9. Módulos conceptuales identificados
{Names + one-line purpose. NOT contracts. Use markers.}

### 10. Riesgos estratégicos detectados
{NOT technical risks per change — strategic project-level risks. Use markers.}

### 11. Secuencia esperada de evolución
{Roadmap at macro level. Use markers.}

### 12. Principio rector
{The non-negotiable guiding principle. Use markers.}

---

## Documento 2 — Misión del Agente y Criterios de Éxito

### 1. Misión del agente
{What the agent's job is in this project.}

### 2. Qué se espera del agente
{Role, stance, attitude.}

### 3. Resultado esperado
{What the agent should produce.}

### 4. Qué sería un buen resultado
{Concrete signals of quality.}

### 5. Qué sería un mal resultado
{Concrete signals of failure.}

### 6. Criterios de éxito del proyecto en esta etapa
{Objective markers that declare this stage done.}

### 7. Qué NO es éxito todavía
{What people sometimes mistake for success but isn't.}

### 8. Prioridad operativa correcta
{The correct order of execution for this stage.}

### 9. Resultado final esperado del trabajo del agente
{The one-sentence version of "when this is done, what exists?"}

---

## Documento 3 — Reglas Obligatorias de Trabajo

### Reglas universales (pre-cargadas)

1. **No inventar definiciones faltantes.** {full text from Universal Base Rules}
2. **No confundir contexto previo con especificación suficiente.** {full text}
3. **Separar siempre dominio, reglas computables y arquitectura.** {full text}
4. **Contratos obligatorios por módulo.** {full text}
5. **Trazabilidad obligatoria.** {full text}
6. **Diferenciar hipótesis, decisión y hecho.** {full text}
7. **Si algo está abierto, dejarlo visible.** {full text}
8. **Priorizar claridad ejecutable sobre completitud artificial.** {full text}

### Reglas de dominio (pendientes de confirmación)

{List domain-specific rules proposed for confirmation — marked as [PENDIENTE] until user agrees.}

9. [PENDIENTE] {Proposed domain rule 1 with rationale}
10. [PENDIENTE] {Proposed domain rule 2 with rationale}

---

## Preguntas Abiertas

### Para Documento 1
1. {Question}
2. {Question}

### Para Documento 2
1. {Question}

### Para Documento 3
1. {Domain rules confirmation questions}

---

## Estado
**Fase**: `draft`. {Brief note on what's needed to reach `final`.}
```

---

## Final Phase Output

```markdown
# Manifiesto — Final: {Project Title}

{Full Doc 1 without markers except [HIPÓTESIS OPERATIVA] where applicable}

---

{Full Doc 2 without markers}

---

{Full Doc 3 — confirmed universal rules + confirmed domain rules}

---

## Compact Rules (para skill registry)

{One line per rule, imperative mood — this is what goes into .atl/skill-registry.md}

- {Rule 1 imperative}
- {Rule 2 imperative}
- ...

## Skill Registry Updated
✅ `.atl/skill-registry.md` actualizado con la sección `Project Manifest Rules`
✅ Engram observation `skill-registry` actualizada (if applicable)

## Estado
**Fase**: `final`. Manifiesto consolidado. Listo para `project-architect`.
```

---

## Structured Response Envelope

Return EXACTLY this format to the orchestrator:

```markdown
## Project Manifest — {phase}

**Project**: {project name}
**Phase**: {discovery | draft | final}
**Artifact store mode**: {engram | openspec | hybrid | none}
**Artifacts**: {list of engram topic keys and/or file paths persisted}

### Executive Summary
{2-3 sentence summary in Spanish of what was analyzed and produced.}

### Questions for User
{If discovery or draft: list questions that need answers. If final: "Ninguna — manifiesto consolidado listo."}

### Next Step
{What happens next:
- discovery: "Respondé las preguntas para avanzar a `draft`. Relanzar este skill con las respuestas en conversation_history."
- draft: "Confirmá los supuestos y reglas de dominio propuestas. Relanzar con respuestas para pasar a `final`."
- final: "Manifiesto listo. Próximo paso recomendado: ejecutar `project-architect` para diseñar la arquitectura técnica respetando el manifest."}

### Risks
- {Strategic risk 1}
- {Strategic risk 2}

### Warnings
{If sdd-init context was not found, warn here.}
{If skill registry could not be updated, warn here.}
```
