ALTER TABLE applications
    ADD CONSTRAINT applications_status_check
    CHECK (status IN ('applied', 'screening', 'interviewing', 'offer', 'accepted', 'rejected', 'withdrawn'));

-- List() orders by applied_at DESC, so back that with an index.
CREATE INDEX applications_applied_at_idx ON applications (applied_at DESC);

-- Stats() and the status filter hit this column.
CREATE INDEX applications_status_idx ON applications (status);
