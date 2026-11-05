-- Internal pricing (Verrechnungspreise, docs/13): the rate a cost type's service is charged out at,
-- and the charge-outs themselves.
--
-- The centre-side engine distributes *actual* costs by keys. This prices an internal service and
-- charges it out: the providing cost centre is relieved by the amount, the receiving one is
-- charged it, so cost still moves between centres without being created or destroyed.
--
-- The vendor's `overhead_calc*` shape is not published, so what is kept here is *data* an operator
-- can change without the engine moving: a rate is a price per unit of a cost type's service, valid
-- over a period, and nothing about it is compiled in.

CREATE TABLE IF NOT EXISTS service_rate (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    cost_type_id bigint NOT NULL REFERENCES cost_type(id),
    -- Minor units per unit of service: 2500 is 25,00 per unit.
    rate_cents   bigint NOT NULL CHECK (rate_cents > 0),
    valid_from   date   NOT NULL,
    valid_to     date,
    created_by   text   NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT service_rate_period CHECK (valid_to IS NULL OR valid_to >= valid_from)
);

CREATE INDEX IF NOT EXISTS service_rate_cost_type_idx ON service_rate (cost_type_id);

-- A rendered service: the provider charged the receiver for a quantity at the rate in force on the
-- day it was rendered. The amount is stored rather than re-derived, because the rate can change
-- afterwards and a charge that was made is a fact about the books.
CREATE TABLE IF NOT EXISTS service_charge (
    id                 bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    provider_center_id bigint NOT NULL REFERENCES cost_center(id),
    receiver_center_id bigint NOT NULL REFERENCES cost_center(id),
    cost_type_id       bigint NOT NULL REFERENCES cost_type(id),
    quantity           bigint NOT NULL CHECK (quantity > 0),
    amount_cents       bigint NOT NULL CHECK (amount_cents > 0),
    charged_on         date   NOT NULL,
    charged_by         text   NOT NULL,
    charged_at         timestamptz NOT NULL DEFAULT now(),
    -- A centre does not charge itself for its own service.
    CONSTRAINT service_charge_not_self CHECK (provider_center_id <> receiver_center_id)
);

CREATE INDEX IF NOT EXISTS service_charge_provider_idx ON service_charge (provider_center_id);
