#!/bin/sh
set -e

if [ -n "$POSTGRES_MULTIPLE_DATABASES" ]; then
  IFS=',' read -r -a dbs <<EOF
$POSTGRES_MULTIPLE_DATABASES
EOF

  for db in "${dbs[@]}"; do
    db="$(echo "$db" | xargs)" # trim spaces
    echo "Creating database: $db"
    psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname postgres \
      -c "CREATE DATABASE \"$db\";"
  done
fi
