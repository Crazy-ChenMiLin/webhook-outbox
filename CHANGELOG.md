# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses
[Semantic Versioning](https://semver.org/). Before 1.0, minor versions may change the
schema or the configuration; such changes are marked **BREAKING**.

## [Unreleased]

### Added

- Tables `webhook_endpoints`, `webhook_messages`, `webhook_deliveries`,
  `webhook_attempts` for PostgreSQL 14 or later, as `schema/postgresql.sql` and as
  `webhook-outbox-relay migrate`. See [docs/schema.md](docs/schema.md).
- `webhook-outbox-relay endpoints add|list|secret|disable|enable`.
