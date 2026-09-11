# Turnstile Progress

## Current

- [ ] Begin Phase 2: ticket inventory and transaction schema

## Completed

- [x] Phase 1 foundation
- [x] Go application entry points for API and migrations
- [x] PostgreSQL connection through bounded `pgxpool`
- [x] Embedded, ordered, transactional database migrations
- [x] Environment-based configuration with validation
- [x] JSON structured logging
- [x] Dockerfile and Docker Compose
- [x] `/health` liveness and database-backed `/ready` readiness
- [x] Signal handling and graceful HTTP shutdown

## Remaining

- [ ] Phases 2-9 from `product_roadmap.md`

## Notes

- PostgreSQL remains system of record; liveness does not depend on database health.
- Migrations use a PostgreSQL advisory lock so concurrent deploys cannot apply the same migration twice.
- `technical_assessment.pdf` is not present in repository. Add it before later phases to confirm source-of-truth requirements.
