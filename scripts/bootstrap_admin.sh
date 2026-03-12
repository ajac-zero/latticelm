#!/bin/bash
# Bootstrap script to promote the first admin user
# Usage: ./scripts/bootstrap_admin.sh <email>

set -e

if [ -z "$1" ]; then
    echo "Usage: $0 <email>"
    echo "Example: $0 admin@example.com"
    exit 1
fi

EMAIL="$1"
DSN="${DATABASE_DSN:-postgresql://neondb_owner:npg_qjCZo9KLbV2R@ep-nameless-surf-aduti1ls-pooler.c-2.us-east-1.aws.neon.tech/neondb?sslmode=require&channel_binding=require}"

echo "Promoting user '$EMAIL' to admin..."

# Use psql for PostgreSQL
if command -v psql &> /dev/null; then
    psql "$DSN" <<SQL
    UPDATE users
    SET role = 'admin', updated_at = NOW()
    WHERE email = '$EMAIL';

    SELECT id, email, name, role, status, created_at
    FROM users
    WHERE email = '$EMAIL';
SQL
    echo "✓ User promoted to admin!"
else
    echo "Error: psql not found. Install PostgreSQL client tools."
    echo ""
    echo "Manual SQL command:"
    echo "  UPDATE users SET role = 'admin', updated_at = NOW() WHERE email = '$EMAIL';"
    exit 1
fi
