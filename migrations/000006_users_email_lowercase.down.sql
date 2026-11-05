-- The original letter case is not stored anywhere, so only the rule can be undone.
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_email_lowercase;
