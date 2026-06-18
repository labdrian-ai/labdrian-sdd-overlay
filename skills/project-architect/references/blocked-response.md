# project-architect — Blocked Response (Option C)

When the state is `blocked-no-manifest` or `blocked-incomplete-manifest`, do NOT generate architecture. Return this response instead. Do NOT persist anything.

```markdown
## Project Architect — Bloqueado

**Estado**: {blocked-no-manifest | blocked-incomplete-manifest}
**Proyecto**: {project name}

### Problema
{If no-manifest: "No encontré un manifiesto del proyecto en engram. Para proponer una arquitectura técnica defendible, necesito contexto estratégico previo: qué es el proyecto, qué NO es, restricciones acordadas, preferencia arquitectónica, reglas obligatorias."}
{If incomplete-manifest: "El manifiesto existe pero tiene secciones críticas pendientes: {list the specific [PENDIENTE] sections blocking architecture}. Sin cerrar esas secciones, cualquier arquitectura que proponga va a estar construida sobre supuestos invisibles — exactamente lo que las reglas del manifest prohíben."}

### Tus opciones

**Opción 1 — Ejecutar `/project-manifest` primero (recomendado)**
Esto es lo correcto para proyectos serios. El manifiesto te lleva de idea difusa a 3 documentos (contexto + misión + reglas) que después uso como base para una arquitectura defendible. Toma más tiempo pero el resultado es auditable y coherente.

**Opción 2 — Pasarme el contexto inline**
Si este proyecto es chico y el manifest completo es overkill, escribime en el chat las siguientes cosas y procedo:
- Naturaleza del proyecto (qué es, qué NO es en esta etapa)
- Restricciones de diseño acordadas (qué NO se puede hacer)
- Preferencia arquitectónica macro (monolito modular, distribuido, etc.)
- 3-5 reglas obligatorias no negociables (ej: "no lookahead temporal", "no microservicios", "todo auditable", etc.)
- Stack técnico si no detecté sdd-init
Voy a marcar todo lo inferido como `[SUPUESTO SIN MANIFEST]` en el documento final para que quede visible la deuda.

**Opción 3 — Cancelar**
Si todavía no tenés claras las respuestas de arriba, mejor cancelar ahora que generar una arquitectura basada en adivinanzas.

### Qué prefieris?
Contestame "1", "2" o "3". Si elegís "2", incluí también el contexto en el mismo mensaje.
```

Return this as the skill output with `status: blocked`.

---

## Input Sufficiency Assessment

Classify the state into one of three outcomes automatically — do NOT ask the user.

| State | Signal | Action |
|-------|--------|--------|
| `blocked-no-manifest` | `manifest_context` NOT found in engram | Offer Option C response |
| `blocked-incomplete-manifest` | `manifest_context` found but marked `[PENDIENTE]` in critical sections (naturaleza, restricciones, preferencia arquitectónica, módulos conceptuales) | Offer Option C response with specific list of pending items blocking architecture |
| `proceed` | Manifest present with enough closed content to design on top of | Execute architecture production |

Critical sections that block if `[PENDIENTE]`:
- naturaleza del proyecto
- restricciones de diseño acordadas
- preferencia arquitectónica (macro)
- módulos conceptuales identificados
