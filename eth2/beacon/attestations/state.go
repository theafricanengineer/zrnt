package attestations

import (
	. "github.com/protolambda/zrnt/eth2/core"
	. "github.com/protolambda/zrnt/eth2/meta"
)

type AttestationData struct {
	// LMD GHOST vote
	BeaconBlockRoot Root

	// FFG vote
	Source Checkpoint
	Target Checkpoint

	// Crosslink vote
	Crosslink Crosslink
}

type PendingAttestation struct {
	AggregationBits CommitteeBits
	Data            AttestationData
	InclusionDelay  Slot
	ProposerIndex   ValidatorIndex
}

type AttestationsState struct {
	PreviousEpochAttestations []*PendingAttestation
	CurrentEpochAttestations  []*PendingAttestation
}

// Rotate current/previous epoch attestations
func (state *AttestationsState) RotateEpochAttestations() {
	state.PreviousEpochAttestations = state.CurrentEpochAttestations
	state.CurrentEpochAttestations = nil
}

func (state *AttestationsState) GetAttestationSlot(meta CrosslinkTimingMeta, attData *AttestationData) Slot {
	epoch := attData.Target.Epoch
	committeeCount := Slot(meta.GetCommitteeCount(epoch))
	offset := Slot((attData.Crosslink.Shard + SHARD_COUNT - meta.GetStartShard(epoch)) % SHARD_COUNT)
	return epoch.GetStartSlot() + (offset / (committeeCount / SLOTS_PER_EPOCH))
}