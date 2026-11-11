# project-manifest — Phase Output Templates

These templates are outlines: render new artifacts and envelopes in English except verbatim user quotes. Do not translate existing stored records. Include only supported content; expose missing evidence rather than inventing it. The parent retains decisions and confirmations.

## Discovery Phase Output

Reformulate the supplied idea, identify known context, assumptions and critical gaps by document, and return the concrete missing-input blocker. Ask at most one blocking question per parent interaction. Do not persist discovery.

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

### Confirmed domain rules

{Only genuinely confirmed domain rules belong here; record unconfirmed proposals as open questions, not rules.}

---

## Open Questions

{Identify unresolved gaps and ask at most one blocking question for this parent interaction.}

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

## Compact Rules (for skill registry)

{Inside the manifest-owned markers, include:

Source: project-manifest skill. Do NOT edit manually.
Triggers: when working on ANY change in this project.
One imperative rule per line (about 15 maximum).
Full rules: `project/{project}/manifest/rules`, `project/{project}/manifest/context`, `project/{project}/manifest/mission`.}

- {Rule 1 imperative}
- {Rule 2 imperative}
- ...

## Skill Registry Updated
✅ `.atl/skill-registry.md` updated with the `Project Manifest Rules` section
✅ Project-scoped `skill-registry` record updated and read back (when applicable)

## Estado
**Fase**: `final`. Manifiesto consolidado. Listo para `project-architect`.
```

---

## Structured Response Envelope

Return `status`, `executive_summary`, `detailed_report`, `artifacts`, `next_recommended`, `risks`, and `skill_resolution` to the parent. Include project, phase, resolved placement (`engram | openspec | hybrid | none`), exact persisted topic keys and/or file paths, the concrete blocker or one blocking question when applicable, and warnings for unavailable context or registry updates. Do not execute the next producer or role.
