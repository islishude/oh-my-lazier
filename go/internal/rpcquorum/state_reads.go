package rpcquorum

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

// StateReadConflictError reports a state read (eth_call, eth_getCode,
// eth_getTransactionCount pending) where a fixed majority of providers gave
// comparable answers but no single answer reached the majority. It is distinct
// from QuorumUnavailableError: the chain answered, the providers disagree.
type StateReadConflictError struct {
	ChainName string
	Operation string
	Votes     map[string]int
}

// Error renders the conflict without any provider payloads or URLs.
func (e *StateReadConflictError) Error() string {
	return fmt.Sprintf("chain %s %s state read has no majority answer across %d distinct results", e.ChainName, e.Operation, len(e.Votes))
}

// IsStateReadConflict reports whether err is a state-read quorum conflict.
func IsStateReadConflict(err error) bool {
	var conflict *StateReadConflictError
	return errors.As(err, &conflict)
}

// EstimateGasConflictError reports gas estimates whose comparable answers
// reached the fixed majority without a usable bounded consensus.
type EstimateGasConflictError struct {
	ChainName string
	Detail    string
}

// Error renders the conflict without provider identities or payloads.
func (e *EstimateGasConflictError) Error() string {
	return fmt.Sprintf("chain %s gas estimate has no bounded majority: %s", e.ChainName, e.Detail)
}

// IsEstimateGasConflict reports whether err is a gas-estimate quorum conflict.
func IsEstimateGasConflict(err error) bool {
	var conflict *EstimateGasConflictError
	return errors.As(err, &conflict)
}

// stateReadProbe is one provider's classified answer to a state read.
type stateReadProbe struct {
	// payload is the successful result (nil-normalized to empty).
	payload []byte
	// revertErr is the provider's original error for a deterministic EVM
	// revert; it stays wrapped so error-chain classification keeps working
	// when a revert wins the vote.
	revertErr error
	// transientErr is any non-comparable failure (timeout, transport, unknown
	// RPC error); it only counts toward unavailability.
	transientErr error
	responded    bool
}

// comparable reports whether the probe carries a votable answer.
func (p stateReadProbe) comparable() bool {
	return p.responded && p.transientErr == nil
}

// stateReadFingerprint keys a comparable answer: success payloads by their
// SHA-256, deterministic reverts by their revert identity.
func (p stateReadProbe) fingerprint() string {
	if p.revertErr != nil {
		return revertFingerprint(p.revertErr)
	}
	sum := sha256.Sum256(p.payload)
	return "s:" + hex.EncodeToString(sum[:])
}

// deterministicRevertIdentity extracts the RPC error code and revert data hex
// that identify an EVM revert deterministically across providers.
func deterministicRevertIdentity(err error) (int, string) {
	code := 0
	var rpcErr rpc.Error
	if errors.As(err, &rpcErr) {
		code = rpcErr.ErrorCode()
	}
	data := ""
	var dataErr rpc.DataError
	if errors.As(err, &dataErr) {
		if payload := dataErr.ErrorData(); payload != nil {
			data = fmt.Sprintf("%v", payload)
		}
	}
	return code, data
}

// isDeterministicRevert reports whether a provider error is a comparable EVM
// execution revert rather than a transport or availability failure. Three
// shapes count: RPC error code 3 (execution reverted), an attached revert
// data payload, or a message-only revert whose RPC error text carries
// "execution reverted" (the shape geth-family endpoints return on some
// call/estimate paths). Everything else stays non-comparable so a quorum
// split can never masquerade as a deterministic revert downstream.
func isDeterministicRevert(err error) bool {
	var rpcErr rpc.Error
	if errors.As(err, &rpcErr) && rpcErr.ErrorCode() == 3 {
		return true
	}
	var dataErr rpc.DataError
	if errors.As(err, &dataErr) && dataErr.ErrorData() != nil {
		return true
	}
	_, messageOnly := revertMessageIdentity(err)
	return messageOnly
}

// revertMessageIdentity extracts the normalized revert text of a message-only
// revert: an RPC error carrying "execution reverted" with neither the code-3
// identity nor attached revert data. The full normalized message is the vote
// identity, so differing revert reasons never merge.
func revertMessageIdentity(err error) (string, bool) {
	var rpcErr rpc.Error
	if !errors.As(err, &rpcErr) {
		return "", false
	}
	message := strings.ToLower(strings.TrimSpace(rpcErr.Error()))
	if !strings.Contains(message, "execution reverted") {
		return "", false
	}
	return message, true
}

// revertFingerprint keys a deterministic revert by what the EVM actually
// returned: canonical revert data when present, the normalized provider
// message otherwise. The RPC error code is deliberately excluded from the
// identity — providers report semantically identical reverts under different
// codes (3 vs -32000), and a shared code with no data must never merge
// distinct revert reasons into a false majority.
func revertFingerprint(err error) string {
	if _, data := deterministicRevertIdentity(err); data != "" {
		return "rd:" + data
	}
	var rpcErr rpc.Error
	if errors.As(err, &rpcErr) {
		return "rm:" + strings.ToLower(strings.TrimSpace(rpcErr.Error()))
	}
	// Not reachable for probes admitted by isDeterministicRevert; the wrapped
	// message includes the provider index, so unknown shapes never merge.
	return "rm:" + strings.ToLower(strings.TrimSpace(err.Error()))
}

// voteStateRead runs one comparable-result vote across every configured
// provider: q = floor(N/2)+1 over the CONFIGURED count. A success payload or a
// deterministic revert reaching q returns immediately and cancels the
// stragglers. Head statuses are never touched by state reads — a historical
// read agreeing across providers must not re-promote a lagging or forked
// provider — only the separate sticky state-conflict dimension moves.
func (c *Client) voteStateRead(ctx context.Context, operation string, read func(ctx context.Context, client *ethclient.Client) ([]byte, error)) ([]byte, error) {
	providers := c.snapshotProviders()
	total := len(providers)
	if total == 0 {
		return nil, errors.New("no rpc providers configured")
	}
	quorum := total/2 + 1

	probes := make([]stateReadProbe, total)
	probeCtx, cancelProbes := context.WithCancel(ctx)
	defer cancelProbes()
	completions := make(chan int, total)
	for index := range providers {
		go func(index int) {
			perProbeCtx, cancel := c.probeContext(probeCtx)
			defer cancel()
			client, err := c.providerClient(perProbeCtx, index)
			if err != nil {
				probes[index] = stateReadProbe{transientErr: wrapProviderOperationError(index, operation, err)}
				completions <- index
				return
			}
			payload, err := read(perProbeCtx, client)
			switch {
			case err == nil:
				if payload == nil {
					payload = []byte{}
				}
				probes[index] = stateReadProbe{payload: payload, responded: true}
			case isDeterministicRevert(err):
				probes[index] = stateReadProbe{revertErr: wrapProviderOperationError(index, operation, err), responded: true}
			default:
				probes[index] = stateReadProbe{transientErr: wrapProviderOperationError(index, operation, err)}
			}
			completions <- index
		}(index)
	}

	votes := make(map[string]int, total)
	winners := make(map[string]stateReadProbe, total)
	fingerprints := make(map[int]string, total)
	comparable := 0
	var transientErrs []error
	majorityFingerprint := ""
	for completed := 0; completed < total; completed++ {
		index := <-completions
		probe := probes[index]
		if !probe.comparable() {
			transientErrs = append(transientErrs, probe.transientErr)
			continue
		}
		comparable++
		fingerprint := probe.fingerprint()
		fingerprints[index] = fingerprint
		votes[fingerprint]++
		if _, ok := winners[fingerprint]; !ok {
			winners[fingerprint] = probe
		}
		if votes[fingerprint] >= quorum && majorityFingerprint == "" {
			majorityFingerprint = fingerprint
			cancelProbes()
		}
	}

	c.applyStateConflicts(fingerprints, majorityFingerprint)
	if majorityFingerprint != "" {
		winner := winners[majorityFingerprint]
		if winner.revertErr != nil {
			return nil, winner.revertErr
		}
		return winner.payload, nil
	}
	if comparable >= quorum {
		// The chain answered but the providers disagree. The aggregate error
		// deliberately wraps NO provider revert: a quorum split must never be
		// classified downstream as a deterministic revert.
		return nil, &StateReadConflictError{ChainName: c.chainName, Operation: operation, Votes: votes}
	}
	return nil, &QuorumUnavailableError{
		ChainName: c.chainName,
		Details:   stateReadFailureDetails(operation, quorum, comparable, transientErrs),
	}
}

// stateReadFailureDetails summarizes an unavailable state read without leaking
// provider URLs or payloads: provider operation errors are already redacted to
// provider indexes when wrapped.
func stateReadFailureDetails(operation string, quorum, comparable int, transientErrs []error) []string {
	details := make([]string, 0, len(transientErrs)+1)
	for _, err := range transientErrs {
		if err != nil {
			details = append(details, err.Error())
		}
	}
	details = append(details, fmt.Sprintf("%s has %d comparable answers, quorum is %d", operation, comparable, quorum))
	return details
}

// applyStateConflicts marks comparable dissenters against the majority answer
// and clears providers that agreed. No majority marks every comparable
// participant. Head statuses are intentionally untouched.
func (c *Client) applyStateConflicts(fingerprints map[int]string, majority string) {
	if len(fingerprints) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for index, fingerprint := range fingerprints {
		if index < 0 || index >= len(c.providers) {
			continue
		}
		c.providers[index].stateConflict = majority == "" || fingerprint != majority
	}
}

// StateAnchor pins one logical multi-read check to a single verified state
// view. Hash addressing is authoritative when set: a tip pin must not follow
// a mid-check reorg onto another branch that happens to occupy the same
// height. Number-only anchors address confirmation-deep reads whose branch
// identity is already settled.
type StateAnchor struct {
	Number *big.Int
	Hash   common.Hash
}

// AnchorFromHead converts a verified quorum head into a hash-pinned anchor.
func AnchorFromHead(head HeadResult) (*StateAnchor, error) {
	if head.Number == nil {
		return nil, errors.New("verified head has no block number")
	}
	anchor := &StateAnchor{Number: new(big.Int).Set(head.Number)}
	if head.Hash != "" {
		anchor.Hash = common.HexToHash(head.Hash)
	}
	return anchor, nil
}

// AnchoredCaller is the minimal read surface for anchor-routed contract calls.
type AnchoredCaller interface {
	CallContract(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error)
	CallContractAtHash(ctx context.Context, call ethereum.CallMsg, blockHash common.Hash) ([]byte, error)
}

// CallAtAnchor routes one read through the anchor: block-hash addressing when
// the anchor carries a hash, block-number addressing for confirmation-deep
// anchors, and the caller's own canonical head anchoring when anchor is nil.
func CallAtAnchor(ctx context.Context, caller AnchoredCaller, call ethereum.CallMsg, anchor *StateAnchor) ([]byte, error) {
	if anchor == nil {
		return caller.CallContract(ctx, call, nil)
	}
	if anchor.Hash != (common.Hash{}) {
		return caller.CallContractAtHash(ctx, call, anchor.Hash)
	}
	return caller.CallContract(ctx, call, anchor.Number)
}

// CallContract executes an eth_call agreed by the fixed configured majority.
// A nil block number anchors the call to the canonical head's block HASH, so a
// same-height minority fork cannot serve its own branch state into the vote;
// an explicit block number is voted at that height (callers anchoring a whole
// logical operation verify the height once through CheckHead).
func (c *Client) CallContract(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
	if blockNumber == nil {
		head, err := c.CheckHead(ctx)
		if err != nil {
			return nil, err
		}
		anchor := common.HexToHash(head.Hash)
		return c.voteStateRead(ctx, "eth_call", func(ctx context.Context, client *ethclient.Client) ([]byte, error) {
			return client.CallContractAtHash(ctx, call, anchor)
		})
	}
	return c.voteStateRead(ctx, "eth_call", func(ctx context.Context, client *ethclient.Client) ([]byte, error) {
		return client.CallContract(ctx, call, blockNumber)
	})
}

// CallContractAtHash executes an eth_call pinned to an explicit block hash and
// agreed by the fixed configured majority. Hash addressing keeps every read of
// a logical multi-read check on the branch its anchor was verified on.
func (c *Client) CallContractAtHash(ctx context.Context, call ethereum.CallMsg, blockHash common.Hash) ([]byte, error) {
	return c.voteStateRead(ctx, "eth_call", func(ctx context.Context, client *ethclient.Client) ([]byte, error) {
		return client.CallContractAtHash(ctx, call, blockHash)
	})
}

// CodeAt returns the contract code agreed by the fixed configured majority,
// voted by code hash with nil and empty code normalized to the same answer.
// A nil block number anchors to the canonical head's block hash.
func (c *Client) CodeAt(ctx context.Context, account common.Address, blockNumber *big.Int) ([]byte, error) {
	if blockNumber == nil {
		head, err := c.CheckHead(ctx)
		if err != nil {
			return nil, err
		}
		anchor := common.HexToHash(head.Hash)
		return c.voteStateRead(ctx, "eth_getCode", func(ctx context.Context, client *ethclient.Client) ([]byte, error) {
			return client.CodeAtHash(ctx, account, anchor)
		})
	}
	return c.voteStateRead(ctx, "eth_getCode", func(ctx context.Context, client *ethclient.Client) ([]byte, error) {
		return client.CodeAt(ctx, account, blockNumber)
	})
}

// CodeAtHash returns the contract code at an explicit block hash agreed by
// the fixed configured majority.
func (c *Client) CodeAtHash(ctx context.Context, account common.Address, blockHash common.Hash) ([]byte, error) {
	return c.voteStateRead(ctx, "eth_getCode", func(ctx context.Context, client *ethclient.Client) ([]byte, error) {
		return client.CodeAtHash(ctx, account, blockHash)
	})
}

// EstimateGas aggregates gas estimates across the fixed configured majority.
// Estimates run on each provider's own latest state (anchoring to a historical
// block would fake reverts for time-sensitive calldata), so exact equality is
// not expected: successful estimates form a bounded set around the upper
// median m — only values in [ceil(m/2), 2m] count — and the maximum in-bounds
// value is returned, so a lone honest high estimate survives while a lone
// hostile one is capped at 2x. A deterministic revert wins only when the same
// revert identity reaches the majority, preserving downstream terminal
// classification. The result must not exceed the canonical block gas limit.
func (c *Client) EstimateGas(ctx context.Context, call ethereum.CallMsg) (uint64, error) {
	header, err := c.HeaderByNumber(ctx, nil)
	if err != nil {
		return 0, err
	}
	blockGasLimit := header.GasLimit

	providers := c.snapshotProviders()
	total := len(providers)
	if total == 0 {
		return 0, errors.New("no rpc providers configured")
	}
	quorum := total/2 + 1

	type gasProbe struct {
		gas          uint64
		revertErr    error
		transientErr error
		responded    bool
	}
	probes := make([]gasProbe, total)
	probeCtx, cancelProbes := context.WithCancel(ctx)
	defer cancelProbes()
	completions := make(chan int, total)
	for index := range providers {
		go func(index int) {
			perProbeCtx, cancel := c.probeContext(probeCtx)
			defer cancel()
			client, err := c.providerClient(perProbeCtx, index)
			if err != nil {
				probes[index] = gasProbe{transientErr: wrapProviderOperationError(index, "eth_estimateGas", err)}
				completions <- index
				return
			}
			gas, err := client.EstimateGas(perProbeCtx, call)
			switch {
			case err == nil:
				probes[index] = gasProbe{gas: gas, responded: true}
			case isDeterministicRevert(err):
				probes[index] = gasProbe{revertErr: wrapProviderOperationError(index, "eth_estimateGas", err), responded: true}
			default:
				probes[index] = gasProbe{transientErr: wrapProviderOperationError(index, "eth_estimateGas", err)}
			}
			completions <- index
		}(index)
	}

	type successVote struct {
		index int
		gas   uint64
	}
	var successes []successVote
	revertVotes := make(map[string]int, total)
	revertWinners := make(map[string]error, total)
	comparable := 0
	var transientErrs []error
	for completed := 0; completed < total; completed++ {
		index := <-completions
		probe := probes[index]
		switch {
		case probe.transientErr != nil:
			transientErrs = append(transientErrs, probe.transientErr)
		case probe.revertErr != nil:
			comparable++
			fingerprint := revertFingerprint(probe.revertErr)
			revertVotes[fingerprint]++
			if _, ok := revertWinners[fingerprint]; !ok {
				revertWinners[fingerprint] = probe.revertErr
			}
		default:
			comparable++
			successes = append(successes, successVote{index: index, gas: probe.gas})
		}
	}

	// Every comparable probe participates in the conflict classification for
	// every outcome: a reverting dissenter against a bounded success majority
	// (or vice versa) must light the sticky state-conflict flag, or the
	// provider-conflict alert misses the faulty endpoint.
	classification := make(map[int]string, total)
	for index, probe := range probes {
		switch {
		case probe.revertErr != nil:
			classification[index] = revertFingerprint(probe.revertErr)
		case probe.responded:
			classification[index] = "s:pending"
		}
	}
	for fingerprint, count := range revertVotes {
		if count >= quorum {
			c.applyStateConflicts(classification, fingerprint)
			return 0, revertWinners[fingerprint]
		}
	}
	if len(successes) >= quorum {
		sorted := make([]uint64, len(successes))
		for i, vote := range successes {
			sorted[i] = vote.gas
		}
		sort.Slice(sorted, func(a, b int) bool { return sorted[a] < sorted[b] })
		median := sorted[len(sorted)/2]
		lower := (median + 1) / 2
		upper := median * 2
		inBounds := make([]uint64, 0, len(successes))
		for _, vote := range successes {
			if vote.gas < lower || vote.gas > upper {
				classification[vote.index] = "s:out-of-bounds"
				continue
			}
			classification[vote.index] = "s:bounded"
			inBounds = append(inBounds, vote.gas)
		}
		c.applyStateConflicts(classification, "s:bounded")
		if len(inBounds) < quorum {
			return 0, &EstimateGasConflictError{ChainName: c.chainName, Detail: fmt.Sprintf("only %d of %d estimates fall within the bounded set, need %d", len(inBounds), len(successes), quorum)}
		}
		result := inBounds[0]
		for _, gas := range inBounds[1:] {
			if gas > result {
				result = gas
			}
		}
		if result > blockGasLimit {
			return 0, &EstimateGasConflictError{ChainName: c.chainName, Detail: fmt.Sprintf("bounded estimate %d exceeds the canonical block gas limit %d", result, blockGasLimit)}
		}
		return result, nil
	}
	if comparable >= quorum {
		// No outcome reached the majority: every comparable probe is marked.
		c.applyStateConflicts(classification, "")
		return 0, &EstimateGasConflictError{ChainName: c.chainName, Detail: fmt.Sprintf("neither %d successes nor any revert identity reached the majority of %d", len(successes), quorum)}
	}
	return 0, &QuorumUnavailableError{
		ChainName: c.chainName,
		Details:   stateReadFailureDetails("eth_estimateGas", quorum, comparable, transientErrs),
	}
}
