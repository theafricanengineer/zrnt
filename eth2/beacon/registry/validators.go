package registry

import (
	. "github.com/protolambda/zrnt/eth2/beacon/validator"
	. "github.com/protolambda/zrnt/eth2/core"
	"github.com/protolambda/zrnt/eth2/util/math"
	"github.com/protolambda/zrnt/eth2/util/ssz"
	"github.com/protolambda/zssz"
	"sort"
)

var RegistryIndicesSSZ = zssz.GetSSZ((*RegistryIndices)(nil))

type ValidatorRegistry []*Validator

func (_ *ValidatorRegistry) Limit() uint64 {
	return VALIDATOR_REGISTRY_LIMIT
}

var ValidatorRegistrySSZ = zssz.GetSSZ((*ValidatorRegistry)(nil))

type ValidatorsState struct {
	Validators ValidatorRegistry
}

func (state *ValidatorsState) IsValidIndex(index ValidatorIndex) bool {
	return index < ValidatorIndex(len(state.Validators))
}

func (state *ValidatorsState) ValidatorCount() uint64 {
	return uint64(len(state.Validators))
}

func (state *ValidatorsState) Validator(index ValidatorIndex) *Validator {
	return state.Validators[index]
}

func (state *ValidatorsState) Pubkey(index ValidatorIndex) BLSPubkey {
	return state.Validators[index].Pubkey
}

func (state *ValidatorsState) ValidatorIndex(pubkey BLSPubkey) (index ValidatorIndex, exists bool) {
	for i, v := range state.Validators {
		if v.Pubkey == pubkey {
			return ValidatorIndex(i), true
		}
	}
	return ValidatorIndexMarker, false
}

func (state *ValidatorsState) WithdrawableEpoch(index ValidatorIndex) Epoch {
	return state.Validators[index].WithdrawableEpoch
}

func (state *ValidatorsState) IsActive(index ValidatorIndex, epoch Epoch) bool {
	return state.Validators[index].IsActive(epoch)
}

func (state *ValidatorsState) GetActiveValidatorIndices(epoch Epoch) RegistryIndices {
	res := make([]ValidatorIndex, 0, len(state.Validators))
	for i, v := range state.Validators {
		if v.IsActive(epoch) {
			res = append(res, ValidatorIndex(i))
		}
	}
	return res
}

func (state *ValidatorsState) ComputeActiveIndexRoot(epoch Epoch) Root {
	indices := state.GetActiveValidatorIndices(epoch)
	return ssz.HashTreeRoot(indices, RegistryIndicesSSZ)
}

func (state *ValidatorsState) GetActiveValidatorCount(epoch Epoch) (count uint64) {
	for _, v := range state.Validators {
		if v.IsActive(epoch) {
			count++
		}
	}
	return
}

func CommitteeCount(activeValidators uint64) uint64 {
	validatorsPerSlot := activeValidators / uint64(SLOTS_PER_EPOCH)
	committeesPerSlot := validatorsPerSlot / TARGET_COMMITTEE_SIZE
	if MAX_COMMITTEES_PER_SLOT < committeesPerSlot {
		committeesPerSlot = MAX_COMMITTEES_PER_SLOT
	}
	if committeesPerSlot == 0 {
		committeesPerSlot = 1
	}
	return committeesPerSlot
}

func (state *ValidatorsState) GetCommitteeCountAtSlot(slot Slot) uint64 {
	return CommitteeCount(state.GetActiveValidatorCount(slot.ToEpoch()))
}

func (state *ValidatorsState) IsSlashed(index ValidatorIndex) bool {
	return state.Validators[index].Slashed
}

// Filters a slice in-place. Only keeps the unslashed validators.
// If input is sorted, then the result will be sorted.
func (state *ValidatorsState) FilterUnslashed(indices []ValidatorIndex) []ValidatorIndex {
	unslashed := indices[:0]
	for _, x := range indices {
		if !state.Validators[x].Slashed {
			unslashed = append(unslashed, x)
		}
	}
	return unslashed
}

func (state *ValidatorsState) GetIndicesToSlash(withdrawal Epoch) (out []ValidatorIndex) {
	for i, v := range state.Validators {
		if v.Slashed && withdrawal == v.WithdrawableEpoch {
			out = append(out, ValidatorIndex(i))
		}
	}
	return
}

func (state *ValidatorsState) GetChurnLimit(epoch Epoch) uint64 {
	return math.MaxU64(MIN_PER_EPOCH_CHURN_LIMIT, state.GetActiveValidatorCount(epoch)/CHURN_LIMIT_QUOTIENT)
}

func (state *ValidatorsState) ExitQueueEnd(epoch Epoch) Epoch {
	// Compute exit queue epoch
	exitQueueEnd := epoch.ComputeActivationExitEpoch()
	for _, v := range state.Validators {
		if v.ExitEpoch != FAR_FUTURE_EPOCH && v.ExitEpoch > exitQueueEnd {
			exitQueueEnd = v.ExitEpoch
		}
	}
	exitQueueChurn := uint64(0)
	for _, v := range state.Validators {
		if v.ExitEpoch == exitQueueEnd {
			exitQueueChurn++
		}
	}
	if exitQueueChurn >= state.GetChurnLimit(epoch) {
		exitQueueEnd++
	}
	return exitQueueEnd
}

func (state *ValidatorsState) ProcessActivationQueue(currentEpoch Epoch, finalizedEpoch Epoch) {
	// Queue validators eligible for activation and not dequeued for activation prior to finalized epoch
	activationQueue := make([]ValidatorIndex, 0)
	for i, v := range state.Validators {
		if v.IsEligibleForActivation(finalizedEpoch) {
			activationQueue = append(activationQueue, ValidatorIndex(i))
		}
	}

	// Order by the sequence of activation_eligibility_epoch setting and then index
	sort.Slice(activationQueue, func(i int, j int) bool {
		aEligible := state.Validators[activationQueue[i]].ActivationEligibilityEpoch
		bEligible := state.Validators[activationQueue[j]].ActivationEligibilityEpoch
		if aEligible == bEligible {
			return activationQueue[i] < activationQueue[j]
		}
		return aEligible < bEligible
	})
	// Dequeued validators for activation up to churn limit (without resetting activation epoch)
	queueLen := uint64(len(activationQueue))
	if churnLimit := state.GetChurnLimit(currentEpoch); churnLimit < queueLen {
		queueLen = churnLimit
	}
	for _, vi := range activationQueue[:queueLen] {
		state.Validators[vi].ActivationEpoch = currentEpoch.ComputeActivationExitEpoch()
	}
}

// Return the total balance sum (1 Gwei minimum to avoid divisions by zero.)
func (state *ValidatorsState) GetTotalStakedBalance(epoch Epoch) (sum Gwei) {
	for _, v := range state.Validators {
		if v.IsActive(epoch) {
			sum += v.EffectiveBalance
		}
	}
	return sum
}

func (state *ValidatorsState) GetAttestersStake(statuses []AttesterStatus, mask AttesterFlag) (out Gwei) {
	for i := range statuses {
		status := &statuses[i]
		b := state.Validators[i].EffectiveBalance
		if status.Flags.HasMarkers(mask) {
			out += b
		}
	}
	if out == 0 {
		return 1
	}
	return
}

func (state *ValidatorsState) GetTotalStake() (out Gwei) {
	for i := range state.Validators {
		out += state.Validators[i].EffectiveBalance
	}
	if out == 0 {
		return EFFECTIVE_BALANCE_INCREMENT
	}
	return
}

func (state *ValidatorsState) EffectiveBalance(index ValidatorIndex) Gwei {
	return state.Validators[index].EffectiveBalance
}

// Return the combined effective balance of an array of validators. (1 Gwei minimum to avoid divisions by zero.)
func (state *ValidatorsState) SumEffectiveBalanceOf(indices []ValidatorIndex) (sum Gwei) {
	for _, vIndex := range indices {
		sum += state.Validators[vIndex].EffectiveBalance
	}
	if sum == 0 {
		return 1
	}
	return sum
}
