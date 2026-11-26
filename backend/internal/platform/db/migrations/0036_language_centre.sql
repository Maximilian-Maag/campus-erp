-- One language throughout: the British spelling of "centre" in the database.
--
-- The KLR tables were created with the American "center" while the payment-order slice and the
-- product's prose already used "centre". The same concept was spelled two ways in one schema.
-- This migration renames the KLR and budget tables, columns, constraints and the one index that
-- carries the word. Historical migrations keep their spelling; the rename is the bridge.
--
-- Every step is guarded: a database that was already renamed by hand (or a re-run after a
-- partial failure) reaches the same end state instead of failing on a missing object. Postgres
-- follows table and column renames through foreign keys and dependent indexes, so the
-- constraints are renamed explicitly only where the name is read by the code (the FK mapping in
-- klr_repo.go names the constraint).

DO $$
DECLARE
    r record;
BEGIN
    IF to_regclass('cost_center') IS NOT NULL AND to_regclass('cost_centre') IS NULL THEN
        ALTER TABLE cost_center RENAME TO cost_centre;
    END IF;
    IF to_regclass('budget_center') IS NOT NULL AND to_regclass('budget_centre') IS NULL THEN
        ALTER TABLE budget_center RENAME TO budget_centre;
    END IF;

    FOR r IN
        SELECT * FROM (VALUES
            ('cost_centre', 'cost_center_pkey', 'cost_centre_pkey'),
            ('cost_centre', 'cost_center_number_key', 'cost_centre_number_key'),
            ('accounting_record', 'accounting_record_cost_center_id_fkey', 'accounting_record_cost_centre_id_fkey'),
            ('allocation', 'allocation_source_center_id_fkey', 'allocation_source_centre_id_fkey'),
            ('allocation', 'allocation_target_center_id_fkey', 'allocation_target_centre_id_fkey'),
            ('allocation_run', 'allocation_run_source_center_id_fkey', 'allocation_run_source_centre_id_fkey'),
            ('cost_unit_key', 'cost_unit_key_source_center_id_fkey', 'cost_unit_key_source_centre_id_fkey'),
            ('cost_unit_run', 'cost_unit_run_source_center_id_fkey', 'cost_unit_run_source_centre_id_fkey'),
            ('service_charge', 'service_charge_provider_center_id_fkey', 'service_charge_provider_centre_id_fkey'),
            ('service_charge', 'service_charge_receiver_center_id_fkey', 'service_charge_receiver_centre_id_fkey'),
            ('budget_centre', 'budget_center_pkey', 'budget_centre_pkey'),
            ('budget_centre', 'budget_center_code_key', 'budget_centre_code_key'),
            ('budget_position', 'budget_position_center_id_fkey', 'budget_position_centre_id_fkey')
        ) AS t(tbl, old_name, new_name)
    LOOP
        IF EXISTS (
            SELECT 1 FROM pg_constraint c
            JOIN pg_class cl ON cl.oid = c.conrelid
            WHERE cl.relname = r.tbl
              AND cl.relnamespace = to_regnamespace(current_schema())
              AND c.conname = r.old_name
        ) THEN
            EXECUTE format('ALTER TABLE %I RENAME CONSTRAINT %I TO %I', r.tbl, r.old_name, r.new_name);
        END IF;
    END LOOP;

    FOR r IN
        SELECT * FROM (VALUES
            ('accounting_record', 'cost_center_id', 'cost_centre_id'),
            ('allocation', 'source_center_id', 'source_centre_id'),
            ('allocation', 'target_center_id', 'target_centre_id'),
            ('allocation_run', 'source_center_id', 'source_centre_id'),
            ('cost_unit_key', 'source_center_id', 'source_centre_id'),
            ('cost_unit_run', 'source_center_id', 'source_centre_id'),
            ('service_charge', 'provider_center_id', 'provider_centre_id'),
            ('service_charge', 'receiver_center_id', 'receiver_centre_id'),
            ('budget_position', 'center_id', 'centre_id'),
            ('dunning_notice', 'claim_reference', 'receivable_reference')
        ) AS t(tbl, old_name, new_name)
    LOOP
        IF EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_schema = current_schema() AND table_name = r.tbl AND column_name = r.old_name
        ) THEN
            EXECUTE format('ALTER TABLE %I RENAME COLUMN %I TO %I', r.tbl, r.old_name, r.new_name);
        END IF;
    END LOOP;

    IF EXISTS (SELECT 1 FROM pg_indexes WHERE schemaname = current_schema() AND indexname = 'accounting_record_cost_center_idx')
       AND NOT EXISTS (SELECT 1 FROM pg_indexes WHERE schemaname = current_schema() AND indexname = 'accounting_record_cost_centre_idx') THEN
        ALTER INDEX accounting_record_cost_center_idx RENAME TO accounting_record_cost_centre_idx;
    END IF;
    IF EXISTS (SELECT 1 FROM pg_indexes WHERE schemaname = current_schema() AND indexname = 'dunning_notice_claim_idx')
       AND NOT EXISTS (SELECT 1 FROM pg_indexes WHERE schemaname = current_schema() AND indexname = 'dunning_notice_receivable_idx') THEN
        ALTER INDEX dunning_notice_claim_idx RENAME TO dunning_notice_receivable_idx;
    END IF;
END $$;
