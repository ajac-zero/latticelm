-- Promote a user to admin role
-- Replace 'your-email@example.com' with the actual email address

-- Step 1: View all users
SELECT id, email, name, role, status, created_at
FROM users
ORDER BY created_at DESC;

-- Step 2: Promote user by email
UPDATE users
SET role = 'admin', updated_at = NOW()
WHERE email = 'your-email@example.com';

-- Step 3: Verify the change
SELECT id, email, name, role, status, updated_at
FROM users
WHERE email = 'your-email@example.com';

-- Alternative: Promote by OIDC subject (if you know it)
-- UPDATE users
-- SET role = 'admin', updated_at = NOW()
-- WHERE oidc_sub = 'user_abc123';

-- Alternative: Promote by user ID
-- UPDATE users
-- SET role = 'admin', updated_at = NOW()
-- WHERE id = '550e8400-e29b-41d4-a716-446655440000';
