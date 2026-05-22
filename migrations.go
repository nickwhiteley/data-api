package dataapi

import "embed"

// MigrationsFS contains plain SQL migration files (no Goose directives) for the
// data_extraction_execution, data_extraction_execution_log, data_extraction_deny, and
// data_extraction_deny_log tables. Host applications incorporate these into their own
// numbered migration sequence and execute them via their own runner.
//
// All SQL files use unqualified table names — host applications must set search_path
// to the target schema before executing, e.g.:
//
//	SET search_path TO core;
//
//go:embed migrations_sql/*.sql
var MigrationsFS embed.FS
