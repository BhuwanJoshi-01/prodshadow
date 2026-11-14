# Guides

## Migration tools

How to use ProdShadow with common migration tools. Each guide shows how to
produce the two SQL inputs ProdShadow needs: the schema before the
migration (for the fixture) and the migration itself.

* [Alembic](alembic.md)
* [Django](django.md)
* [Flyway and Liquibase](flyway.md)
* [golang-migrate and goose](go-migration-tools.md)
* [Prisma Migrate](prisma.md)

## Patterns

* [Safe migration patterns](safe-migration-patterns.md): recipes for common
  schema changes, with the rules that report the risky version

## Modelling

* [Modelling a workload from production statistics](modelling-traffic.md)
