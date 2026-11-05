-- From now on the app lower-cases every email before saving or searching it
-- (util.NormalizeEmail). This migration converts the rows that already exist.

-- 1. If two accounts differ only by letter case (Test@mail.com and test@mail.com),
--    lower-casing them would break the UNIQUE index. Merging two people's accounts
--    is a human decision, so stop with a clear message instead of guessing.
--    golang-migrate marks version 6 "dirty" when a migration fails. Nothing has been
--    changed at that point, so after fixing the emails it is safe to reset the mark.
DO $$
DECLARE
    clashes text;
BEGIN
    SELECT string_agg(e, ', ') INTO clashes
    FROM (SELECT lower(btrim(email)) AS e
          FROM users
          GROUP BY 1
          HAVING count(*) > 1) d;

    IF clashes IS NOT NULL THEN
        RAISE EXCEPTION 'accounts whose emails differ only by letter case: %. Change one email of each pair by hand, then run "UPDATE schema_migrations SET version = 5, dirty = false;" and restart the app.', clashes;
    END IF;
END $$;

-- 2. Convert the rest.
UPDATE users SET email = lower(btrim(email)) WHERE email <> lower(btrim(email));

-- 3. The database itself now refuses a mixed-case email, even from a manual INSERT.
ALTER TABLE users ADD CONSTRAINT users_email_lowercase CHECK (email = lower(email));
