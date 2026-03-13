-- Remove duplicate users per email, keeping the oldest record
DELETE FROM users
WHERE id IN (
    SELECT u1.id FROM users u1
    WHERE EXISTS (
        SELECT 1 FROM users u2
        WHERE u2.email = u1.email
        AND (u2.created_at < u1.created_at OR (u2.created_at = u1.created_at AND u2.id < u1.id))
    )
);

-- Replace the non-unique email index with a unique one
DROP INDEX IF EXISTS idx_users_email;
CREATE UNIQUE INDEX idx_users_email ON users(email);
