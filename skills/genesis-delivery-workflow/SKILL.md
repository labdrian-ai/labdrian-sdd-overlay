---
name: genesis-delivery-workflow
description: >
  Genesis-only delivery workflow rules for commits, push, Bitbucket PRs,
  chained PRs, TypeORM migrations, idempotent seeds, and production deploy DB
  gates. Trigger: when committing, pushing, creating Genesis PRs, planning
  work-unit/chained PR slices, changing entities or schema, adding seeds, or
  preparing deploy database gates in the Genesis repo.
license: Apache-2.0
metadata:
  author: genesis
  version: "1.0"
  project: genesis
---

# Genesis Delivery Workflow

Use this skill only in the Genesis repository. It captures project-specific
delivery rules that override generic global commit, PR, migration, seed, and
deployment advice.

## Priority Rules

1. Genesis `AGENTS.md` has precedence over global skills and generic tooling
   guidance.
2. Do not use GitHub PR tooling for Genesis: no `gh pr`, GitHub labels,
   GitHub templates, or GitHub Actions assumptions.
3. Use `infra/scripts/bb-pr.sh` for Bitbucket PR operations. Push the source
   branch to `origin` before creating a PR.
4. Feature and fix PRs target `staging`. `main` is only for production
   promotion from `staging`.
5. Do not push directly to `main`. Do not push directly to `staging` unless a
   maintainer explicitly approves it for the current task.
6. Commits use Conventional Commits. Never add AI attribution. Do not use
   force push or `--no-verify`.
7. For large or SDD-scoped changes, reviewable work-unit commits and chained PR
   slices override the one-commit default.
8. SDD artifacts remain Engram-only. Do not create filesystem `openspec/`
   artifacts unless Genesis maintainers explicitly approve a temporary override.

## Commit Grouping

| Situation | Commit Strategy |
| --- | --- |
| Small cohesive bugfix or feature | One Conventional Commit with tests/docs if applicable. |
| Multiple unrelated concerns | Split by concern; each commit should be reviewable and buildable. |
| Large implementation or SDD change | Use work-unit commits and consider chained PRs. |
| Schema/entity plus application code | Keep migration with the schema/entity change it supports. |
| Generated or mechanical follow-up | Commit with the source change unless it would obscure review. |

## PR Target

| Situation | Target |
| --- | --- |
| Feature PR | `staging` |
| Fix PR | `staging` |
| Chained PR slice | Previous slice branch, or `staging` for the first slice |
| Production promotion | `main`, source must be `staging` |
| Direct branch push to `main` or `staging` | Not allowed without explicit maintainer approval |

## Bitbucket PR Commands

Push the source branch before creating the PR:

```bash
git push origin <branch>
```

Create feature or fix PRs with the repo script:

```bash
./infra/scripts/bb-pr.sh create \
  --source "<branch>" \
  --target "staging" \
  --title "<conventional title>" \
  --body "<markdown body>"
```

For available operations and required environment variables, run:

```bash
./infra/scripts/bb-pr.sh --help
```

Do not invent unsupported flags. If the script does not support an operation,
report that constraint instead of switching to GitHub tooling.

## Migration And Seed Requirement

| Change | Required Action |
| --- | --- |
| Entity column, relation, index, constraint, enum, or table change | Add a TypeORM migration. |
| Raw schema change outside an entity | Add a TypeORM migration. |
| Required default/reference data | Add a production-safe idempotent seed. |
| Optional local demo data | Keep it dev/demo-only; never run automatically in production. |
| Destructive data reset or sample replacement | Never run automatically in production. |

## Seed Type

| Seed Type | Production Behavior |
| --- | --- |
| Required reference/default data | Idempotent and production-safe. |
| Documentation template seed | Insert-only/no overwrite by default in production. |
| Manual maintenance seed | May update existing data only with explicit maintenance approval. |
| Dev/demo seed | Not automatic in production. |
| Destructive/reset seed | Not allowed in production automation. |

## Production DB Gate

Production deployment must gate database work before backend restart:

1. Require explicit backup confirmation.
2. Run pending TypeORM migrations.
3. Run required production-safe seeds.
4. Restart the backend only after database work succeeds.
5. Run a healthcheck after restart.

Do not enable TypeORM `migrationsRun` for normal production application startup.
Production migrations are an explicit deploy step, not an app boot side effect.
