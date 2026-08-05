#!/bin/bash
set -e

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
    CREATE USER demouser WITH PASSWORD 'demopass';
    CREATE DATABASE demodb OWNER demouser;
EOSQL

psql -v ON_ERROR_STOP=1 --username demouser --dbname demodb <<-EOSQL
    CREATE TABLE deployments (
        id BIGSERIAL PRIMARY KEY,
        time TIMESTAMPTZ NOT NULL,
        service TEXT NOT NULL,
        environment TEXT NOT NULL,
        duration_seconds DOUBLE PRECISION NOT NULL,
        success BOOLEAN NOT NULL,
        rollback BOOLEAN NOT NULL DEFAULT FALSE
    );

    CREATE TABLE incidents (
        id BIGSERIAL PRIMARY KEY,
        time TIMESTAMPTZ NOT NULL,
        severity TEXT NOT NULL,
        service TEXT NOT NULL,
        time_to_detect_minutes DOUBLE PRECISION NOT NULL,
        time_to_resolve_minutes DOUBLE PRECISION NOT NULL
    );

    CREATE TABLE build_metrics (
        id BIGSERIAL PRIMARY KEY,
        time TIMESTAMPTZ NOT NULL,
        repository TEXT NOT NULL,
        branch TEXT NOT NULL,
        duration_seconds DOUBLE PRECISION NOT NULL,
        test_count INTEGER NOT NULL,
        tests_failed INTEGER NOT NULL,
        coverage_pct DOUBLE PRECISION NOT NULL
    );

    CREATE INDEX idx_deployments_time ON deployments (time);
    CREATE INDEX idx_incidents_time ON incidents (time);
    CREATE INDEX idx_build_metrics_time ON build_metrics (time);
EOSQL
