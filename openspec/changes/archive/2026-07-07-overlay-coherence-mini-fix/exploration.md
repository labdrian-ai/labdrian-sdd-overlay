# Exploration Notes: Overlay Coherence Mini-Fix

## Summary

This change focused on aligning the wrapper command surface, hook status behavior, canonical GADU wording, and documentation so the overlay remains coherent across generated artifacts.

## Findings

- `bin/labdrian-overlay` is the user-facing wrapper and must remain a thin bridge.
- `status-hooks` should be read-only; build/deploy belongs in `install-hooks`.
- The canonical GADU body is the single source for the three generated artifacts.
- Documentation drift was limited to command behavior and artifact inventory.
