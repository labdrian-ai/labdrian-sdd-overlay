# project-manifest — Universal Base Rules

These 8 rules are pre-loaded into every manifest Doc 3. They apply to ANY project regardless of domain. The `draft` and `final` phases MUST include these as the foundation of Doc 3, adding domain-specific rules on top.

1. **No inventar definiciones faltantes.** Si una regla operativa no está formalizada de forma suficiente para ser implementada de manera auditable, marcarla como pendiente, insuficiente o variante a comparar. No cerrarla por criterio propio.
2. **No confundir contexto previo con especificación suficiente.** El hecho de que exista código previo o conversaciones anteriores no significa que exista una regla implementable. Si algo no está expresado como condición verificable, sigue siendo una definición incompleta.
3. **Separar siempre dominio, reglas computables y arquitectura.** Nunca mezclar qué objetos existen (dominio), qué debe cumplirse para tomar una decisión (reglas), y cómo se ejecuta eso (arquitectura/runtime).
4. **Contratos obligatorios por módulo.** Ningún módulo puede quedar definido solo por nombre. Cada módulo debe tener explícitos: propósito, inputs, outputs, invariantes, decisiones autorizadas, decisiones prohibidas, dependencias permitidas y prohibidas.
5. **Trazabilidad obligatoria.** Toda decisión debe poder reconstruirse: qué contexto usó, qué opciones evaluó, por qué eligió, qué descartó, qué variante experimental aplicó, resultado final. No sirven logs vagos ni narrativos.
6. **Diferenciar hipótesis, decisión y hecho.** Distinguir siempre entre hipótesis operativa, definición cerrada, decisión de diseño, hecho validado y suposición no confirmada. No mezclar esos niveles.
7. **Si algo está abierto, dejarlo visible.** Cuando una definición no esté cerrada, marcarla explícitamente abierta, explicar por qué importa, proponer alternativas comparables si corresponde, señalar impacto. Nunca ocultar ambigüedad para "cerrar" mejor un documento.
8. **Priorizar claridad ejecutable sobre completitud artificial.** Es preferible una arquitectura incompleta pero honesta, que una arquitectura completa en apariencia pero construida sobre supuestos invisibles.

---

## Domain Extension (draft phase)

At the end of the draft, ask the user if they want to add domain-specific rules. Offer 2-3 examples based on what you detected:

- "Si este proyecto maneja series temporales o trading, ¿querés una regla explícita sobre causalidad temporal y prohibición de lookahead/leakage?"
- "Si maneja datos personales o salud, ¿querés reglas explícitas de privacidad, consentimiento y minimización de datos?"
- "Si tiene componentes determinísticos críticos (criptografía, consensos, matching engines), ¿querés una regla de determinismo estricto?"
- "Si hay restricciones regulatorias (financiero, legal, medical devices), ¿querés reglas explícitas de compliance?"

Do NOT add these rules without user confirmation. Document 3 must only contain rules the user has explicitly agreed to.

---

## Marking Convention (MANDATORY for draft phase)

- `[DEFINIDO]` — confirmed by user or clearly stated in input
- `[SUPUESTO]` — inferred by you, needs user confirmation
- `[HIPÓTESIS]` — operating hypothesis (may change during backtesting/validation)
- `[PENDIENTE]` — open question, blocks finalization

Every section of every document MUST use at least one marker somewhere. Do NOT write unmarked prose that hides uncertainty.

If any section is too uncertain to draft, write `[PENDIENTE] Sección no redactable todavía — falta: {what is needed}` instead of guessing.

---

## Discovery Phase — Topics to Probe

Select only what's relevant — do NOT dump the full list.

Context (Doc 1):
- Nature of the project (what it is, what it is NOT in this stage)
- Initial universe / scope
- Conceptual core (the main ideas/pillars the project stands on)
- Current state of knowledge (what's defined, what's missing)
- Operating assumptions (hypotheses, not truths)
- Central problem (the real uncertainty)
- Agreed design constraints
- Architectural preference (macro only: monolith, modular, distributed, etc.)
- Conceptual modules already identified
- Strategic risks
- Expected evolution sequence
- Guiding principle

Mission (Doc 2):
- What the user expects from the agent
- What counts as a good result
- What counts as a bad result
- Success criteria in this stage
- What is NOT success yet
- Operational priority (order of execution)

Rules (Doc 3):
- Hard constraints the agent must respect
- Things the agent must NEVER invent
- Domain-specific rules (e.g., temporal causality in trading, privacy in healthcare, determinism in cryptography)
- Separation of concerns the project requires
