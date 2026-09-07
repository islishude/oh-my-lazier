-- Recovery clocks are independent of general row updates. Historical evidence
-- is initialized lazily from immutable attempt history by the recovery worker.
ALTER TABLE tx_outbox
  ADD COLUMN first_broadcast_at timestamptz,
  ADD COLUMN last_seen_at timestamptz,
  ADD COLUMN absent_since timestamptz,
  ADD COLUMN visibility_checked_at timestamptz,
  ADD COLUMN next_visibility_at timestamptz,
  ADD COLUMN next_recovery_at timestamptz,
  ADD COLUMN recovery_reason text NOT NULL DEFAULT '' CHECK (recovery_reason IN
    ('', 'fee_cap', 'replacement_exhausted', 'rpc_unavailable', 'transaction_unseen', 'evidence_missing')),
  ADD COLUMN recovery_since timestamptz,
  ADD COLUMN recovery_logged_at timestamptz,
  ADD COLUMN recovery_detail jsonb,
  ADD COLUMN recovery_lease_token uuid,
  ADD COLUMN recovery_lease_until timestamptz,
  ADD COLUMN replay_authorized boolean NOT NULL DEFAULT false;

CREATE TABLE tx_recovery_lanes (
  chain_eid integer NOT NULL REFERENCES chains(eid),
  signer_id text NOT NULL,
  observed_nonce bigint,
  nonce_observed_at timestamptz,
  nonce_changed_at timestamptz,
  max_inflight integer NOT NULL CHECK (max_inflight > 0),
  PRIMARY KEY (chain_eid, signer_id)
);
