---
name: Feature issue
about: A self-contained, TDD-first unit of roadmap work for Intela
title: "[Scope] <expressive summary>"
labels: ''
assignees: ''
---

<!--
  Titles are prefixed by scope: [Backend] [Frontend] [Database] [DevOps] [Docs].
  Prose is English. Domain nouns stay untranslated per CLAUDE.md:
  obra, titular, reparto, recaudo, declaracion, liquidacion, anticipo, ONI, bolsa, valor punto.
  Max 2 labels. One issue = one PR = one `feature/<slug>` branch.
  Citations: RD = Reglamento de Distribucion IX, RT = Tarifas VI, RS = Socios, RA = Anticipos.
-->

## Metadata

- **Milestone:** Sprint N
- **Labels:** `label-a`, `label-b` <!-- max 2 -->
- **Assignee:** @handle
- **Blocked by:** #<id>, or _none_
- **Architectural component:** `internal/dominio/<module>` | `internal/aplicacion` | `internal/infraestructura/<adapter>` | `web/` | `cmd/<entry>` | `.github/`

## Problem / Feature Description

<!--
  Business context: which OE / KR / reglamento rule this serves, and why REDES SGC needs it.
  Technical context: what is missing in the codebase today. 3-6 sentences.
-->

## Proposed Solution

<!--
  High-level approach that respects the dependency rule (ADR 0002/0012): nothing in
  internal/dominio/ names anything outside it. Name the ports declared (in aplicacion) or
  implemented (in infraestructura), the layer each new type lives in, and any ADR the work
  must honour. No line-by-line design.
-->

## Acceptance Criteria

- [ ] <verifiable behavioural condition tied to an acceptance criterion in the deliverable doc>
- [ ] <...>
- [ ] Boundary holds: `golangci-lint run --enable-only=depguard ./...` is green
- [ ] `make verificar` is green (backend) / `npm --prefix web run build` + tests green (frontend)

## Testing & TDD Plan

<!-- Tests are written before the implementation. -->

- **Unit:** <pure-domain, table-driven cases; for the reparto engine: golden files built from the reglamento's own worked examples>
- **Integration:** <testcontainers-go against real Postgres for adapters; HTTP handler tests through the chi router>
- **Contract:** <`api/openapi.yaml` updated and `redocly lint` green; regenerated frontend types where the endpoint shape changes>

## Alternatives Considered

<!-- 1-3 bullets: option -> why not. -->

## Additional Context

<!-- Edge cases, security considerations, reglamento numerals (RD/RT/RS/RA), linked ADRs, Figma frames for frontend work. -->
