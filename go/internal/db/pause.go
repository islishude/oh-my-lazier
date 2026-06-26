package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
)

// PauseChain marks a chain as paused after a chain-wide safety fault.
func (s *Store) PauseChain(ctx context.Context, eid uint32) error {
	if eid == 0 {
		return errors.New("chain eid is required")
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE chains
		SET paused = true
		WHERE eid = $1
	`, eid)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("chain %d was not found", eid)
	}
	return nil
}

// PausePathwayForPacket marks the configured packet pathway as paused.
func (s *Store) PausePathwayForPacket(ctx context.Context, guid common.Hash) error {
	if guid == (common.Hash{}) {
		return errors.New("packet guid is required")
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE pathways AS pathway
		SET paused = true
		FROM packets AS packet
		WHERE
			packet.guid = $1
			AND pathway.src_eid = packet.src_eid
			AND pathway.dst_eid = packet.dst_eid
			AND pathway.src_oapp = packet.sender
			AND pathway.dst_oapp = packet.receiver
	`, guid.Bytes())
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("pathway for packet %s was not found", guid)
	}
	return nil
}
