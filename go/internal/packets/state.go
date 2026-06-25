package packets

// ExecutorState is the durable state of a packet in the executor workflow.
type ExecutorState string

const (
	// ExecutorNew is the initial executor state before assignment is confirmed.
	ExecutorNew ExecutorState = "NEW"
	// ExecutorAssigned records that the executor contract assigned the job.
	ExecutorAssigned ExecutorState = "ASSIGNED"
	// ExecutorWaitingDVNVerification waits for required DVNs to verify the packet.
	ExecutorWaitingDVNVerification ExecutorState = "WAITING_DVN_VERIFICATION"
	// ExecutorVerifiable means the destination ULN reports the packet can be committed.
	ExecutorVerifiable ExecutorState = "VERIFIABLE"
	// ExecutorCommitTxEnqueued records that commitVerification was enqueued.
	ExecutorCommitTxEnqueued ExecutorState = "COMMIT_TX_ENQUEUED"
	// ExecutorCommitted records that verification is committed on the destination chain.
	ExecutorCommitted ExecutorState = "COMMITTED"
	// ExecutorExecutable means the endpoint reports lzReceive can be executed.
	ExecutorExecutable ExecutorState = "EXECUTABLE"
	// ExecutorLzReceiveTxEnqueued records that the delivery transaction was enqueued.
	ExecutorLzReceiveTxEnqueued ExecutorState = "LZ_RECEIVE_TX_ENQUEUED"
	// ExecutorDelivered records a successful lzReceive delivery.
	ExecutorDelivered ExecutorState = "DELIVERED"
	// ExecutorLzReceiveFailed records a destination delivery failure.
	ExecutorLzReceiveFailed ExecutorState = "LZ_RECEIVE_FAILED"
	// ExecutorManualReview marks packets requiring operator review.
	ExecutorManualReview ExecutorState = "MANUAL_REVIEW"
)

// DVNState is the durable state of a packet in the DVN verification workflow.
type DVNState string

const (
	// DVNNew is the initial DVN state before assignment is confirmed.
	DVNNew DVNState = "NEW"
	// DVNAssigned records that OpenDVN was assigned for the packet.
	DVNAssigned DVNState = "ASSIGNED"
	// DVNWaitingConfirmations waits for the configured source-chain confirmations.
	DVNWaitingConfirmations DVNState = "WAITING_CONFIRMATIONS"
	// DVNQuorumChecking records that RPC quorum verification is in progress.
	DVNQuorumChecking DVNState = "QUORUM_CHECKING"
	// DVNReadyToVerify means quorum checks passed and verification could be submitted.
	DVNReadyToVerify DVNState = "READY_TO_VERIFY"
	// DVNWouldVerify records a shadow-mode verification report.
	DVNWouldVerify DVNState = "WOULD_VERIFY"
	// DVNVerifyTxEnqueued records that an active verification transaction was enqueued.
	DVNVerifyTxEnqueued DVNState = "VERIFY_TX_ENQUEUED"
	// DVNVerified records an on-chain successful verification.
	DVNVerified DVNState = "VERIFIED"
	// DVNQuorumConflict records conflicting RPC evidence.
	DVNQuorumConflict DVNState = "QUORUM_CONFLICT"
	// DVNReorgDetected records a source-chain reorg affecting the packet.
	DVNReorgDetected DVNState = "REORG_DETECTED"
	// DVNManualReview marks DVN jobs requiring operator review.
	DVNManualReview DVNState = "MANUAL_REVIEW"
)

// Packet is the normalized cross-chain message identity shared by worker workflows.
type Packet struct {
	GUID        string
	SrcEID      uint32
	DstEID      uint32
	Sender      string
	PayloadHash string
}
