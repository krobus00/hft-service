#!/bin/sh
set -e

if [ -n "$POSTGRES_MULTIPLE_DATABASES" ]; then
  echo "$POSTGRES_MULTIPLE_DATABASES" | tr ',' '\n' | while IFS= read -r db; do
    db="$(echo "$db" | xargs)" # trim spaces
    [ -n "$db" ] || continue
    echo "Creating database: $db"
    psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname postgres \
      -c "CREATE DATABASE \"$db\";"
  done
fi
