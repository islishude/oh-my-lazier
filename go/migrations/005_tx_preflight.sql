-- Observation tokens order concurrent RPC checks without using worker clocks.
CREATE SEQUENCE tx_preflight_observations;

ALTER TABLE tx_outbox
  ADD COLUMN preflight_observation_id BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN preflight_observed_at TIMESTAMPTZ,
  ADD COLUMN preflight_failure_since TIMESTAMPTZ,
  ADD COLUMN preflight_reason TEXT NOT NULL DEFAULT '' CHECK (preflight_reason IN
    ('', 'invalid_priority_fee', 'fee_cap', 'fee_quote_error', 'estimate_gas_error')),
  ADD COLUMN preflight_error TEXT NOT NULL DEFAULT '',
  ADD CHECK ((preflight_reason = '') = (preflight_failure_since IS NULL));

CREATE INDEX idx_tx_outbox_preflight_blocked ON tx_outbox(chain_eid, signer_id)
  WHERE status = 'queued' AND nonce IS NULL AND preflight_failure_since IS NOT NULL;
