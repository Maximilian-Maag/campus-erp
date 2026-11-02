-- The stored document of a dunning notice (docs/24). The notice row keeps a reference
-- to the artefact; the bytes live in the document store.
ALTER TABLE dunning_notice ADD COLUMN IF NOT EXISTS document_key text;
