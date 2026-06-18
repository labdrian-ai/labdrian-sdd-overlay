# Roadmap SDD — {Project Name}

**Fecha**: {current date}
**Versión**: {1 if first, n+1 if update}
**Basado en**:
- Manifest contexto: `project/{project}/manifest/context`
- Manifest reglas: `project/{project}/manifest/rules`
- Arquitectura: `project/{project}/architect/final`
- Historia SDD existente: {list archived/completed changes, or "ninguna"}

---

## Resumen de Secuencia

| Orden | SDD-id | Estado | Depende de | Objetivo |
|-------|--------|--------|-----------|----------|
| 0 | {SDD-id} | completado | — | {foundational, ya construido} |
| 1 | {SDD-id} | planificado | {SDD-id} | {goal} |

---

## Detalle por SDD

### {SDD-id} — {Goal corto}

- **Estado**: completado | en curso | bloqueado | diferido | superseded | planificado
- **Objetivo**: {one sentence describing what this change delivers}
- **Derivado de**: {>=1 citation from manifest/architecture/existing SDD — if none, write `[PENDIENTE DE DECISIÓN]`}
- **Dependencias**: {SDD-ids that must land first, or "ninguna"}
- **Evidencia de aceptación**: {observable signal that proves this SDD is done}
- **Riesgo si se hace antes de tiempo**: {concrete risk of premature execution}
- **Comando de entrada SDD**: `/sdd-new {change-id}`

#### Tracking (actualizar a lo largo del ciclo)

| Campo | Valor |
|-------|-------|
| Estimado (antes de empezar) | {…} |
| Esfuerzo de implementación (real) | {… o `[PENDIENTE]`} |
| Esfuerzo de verificación (real) | {… o `[PENDIENTE]`} |
| Duración review humano | {… o `[PENDIENTE]`} |
| Hallazgos del review | {… o `[PENDIENTE]`} |
| Fixes post-review | {… o `[PENDIENTE]`} |
| Tiempo total (inicio → cierre aprobado) | {… o `[PENDIENTE]`} |
| Aprobación/cierre (fecha o decisión) | {… o `[PENDIENTE]`} |
| Desvíos del plan original (y por qué) | {… o "ninguno"} |
| Impacto en la secuencia (reordenamiento/scope) | {… o "ninguno"} |

---

{Repeat the detail block per SDD. Foundational/completed items first, then the planned sequence in dependency order.}
