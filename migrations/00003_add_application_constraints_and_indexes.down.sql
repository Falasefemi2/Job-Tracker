DROP INDEX IF EXISTS applications_status_idx;
DROP INDEX IF EXISTS applications_applied_at_idx;
ALTER TABLE applications DROP CONSTRAINT IF EXISTS applications_status_check;
