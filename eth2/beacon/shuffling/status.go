package shuffling

import (
	. "github.com/protolambda/zrnt/eth2/core"
	. "github.com/protolambda/zrnt/eth2/meta"
	"github.com/protolambda/zrnt/eth2/util/shuffle"
)

type ShufflingComputeReq interface {
	VersioningMeta
	RandomnessMeta
	ActiveIndicesMeta
	CrosslinkTimingMeta
}

// TODO: may want to pool this to avoid large allocations in mainnet.
type ShufflingStatus struct {
	Previous *ShufflingEpoch
	Current  *ShufflingEpoch
}

func (state *ShufflingState) LoadShufflingStatus(meta ShufflingComputeReq) *ShufflingStatus {
	return &ShufflingStatus{
		Previous: state.LoadShufflingEpoch(meta, meta.PreviousEpoch()),
		Current: state.LoadShufflingEpoch(meta, meta.Epoch()),
	}
}

// With a high amount of shards, or low amount of validators,
// some shards may not have a committee this epoch.
type ShufflingEpoch struct {
	Shuffling  []ValidatorIndex              // the active validator indices, shuffled into their committee
	Committees [SHARD_COUNT][]ValidatorIndex // slices of Shuffling, 1 per slot. Committee can be nil slice.
}

func (state *ShufflingState) LoadShufflingEpoch(meta ShufflingComputeReq, epoch Epoch) *ShufflingEpoch {
	shep := new(ShufflingEpoch)
	currentEpoch := meta.Epoch()
	previousEpoch := meta.PreviousEpoch()
	nextEpoch := currentEpoch + 1

	if !(previousEpoch <= epoch && epoch <= nextEpoch) {
		panic("could not compute shuffling for out of range epoch")
	}

	seed := state.GetSeed(meta, epoch)
	activeIndices := meta.GetActiveValidatorIndices(epoch)
	shuffle.UnshuffleList(activeIndices, seed)
	shep.Shuffling = activeIndices

	validatorCount := uint64(len(activeIndices))
	committeeCount := meta.GetCommitteeCount(epoch)
	startShard := meta.GetStartShard(epoch)
	for shard := Shard(0); shard < SHARD_COUNT; shard++ {
		index := uint64((shard + SHARD_COUNT - startShard) % SHARD_COUNT)
		startOffset := (validatorCount * index) / committeeCount
		endOffset := (validatorCount * (index + 1)) / committeeCount
		committee := shep.Shuffling[startOffset:endOffset]
		shep.Committees[shard] = committee
	}
	return shep
}