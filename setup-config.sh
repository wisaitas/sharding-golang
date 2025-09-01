#!/bin/bash
set -e

echo "Setting up PostgreSQL configuration..."

# Copy custom configuration files to the data directory
cp /docker-entrypoint-initdb.d/02-pg_hba.conf /var/lib/postgresql/data/pg_hba.conf
cp /docker-entrypoint-initdb.d/03-postgresql.conf /var/lib/postgresql/data/postgresql.conf

# Set proper ownership
chown postgres:postgres /var/lib/postgresql/data/pg_hba.conf
chown postgres:postgres /var/lib/postgresql/data/postgresql.conf

echo "Configuration files copied successfully"

# Reload configuration
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
    SELECT pg_reload_conf();
EOSQL

echo "PostgreSQL configuration reloaded"
