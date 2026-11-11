-- A collection run may collect under more than one sequence type.
--
-- The sequence type belongs to the payment instruction (PmtInf), not to the message, so a run that
-- gathers a first collection and a recurring one can be a single pain.008 file with two
-- instructions instead of two runs and two files. Such a run has no single type, which it records as
-- an empty string rather than as a NULL: the column stays NOT NULL, every existing filter keeps
-- working, and '' cannot be mistaken for a type a mandate could carry (mandates keep the four-value
-- check of 0017).
ALTER TABLE direct_debit_run DROP CONSTRAINT IF EXISTS direct_debit_run_sequence_type_check;

ALTER TABLE direct_debit_run
    ADD CONSTRAINT direct_debit_run_sequence_type_check
    CHECK (sequence_type = '' OR sequence_type IN ('FRST', 'RCUR', 'OOFF', 'FNAL'));
