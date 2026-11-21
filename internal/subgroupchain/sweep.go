package subgroupchain

import "fmt"

// --------------------------------------------------------------------------------------
//
// Warp sweep evaluation — the universal objective measurement
//
// Every memory-hierarchy objective (GMEM sector count, SMEM bank
// distinctness) is measured the same way: sweep the relevant (lane) variable
// over [0, bound) with every other concurrency variable pinned at 0 and
// reduce the byte offsets to a predicate. This is exactly the per-warp
// contract: a warp's threads differ only in the lane variable.
//
// --------------------------------------------------------------------------------------

const (
	// GmemSectorBytes is the GMEM sector granularity (NVIDIA: 32B).
	GmemSectorBytes = 32
	// SmemBankCount is the number of shared-memory banks per transaction
	// (NVIDIA: 32).
	SmemBankCount = 32
	// SmemBankWidthBytes is the bank word width (NVIDIA: 4B).
	SmemBankWidthBytes = 4
)

// SweepStats reports a warp sweep's byte-offset footprint.
type SweepStats struct {
	// Lo/Hi/Count describe the element-offset set: Count distinct elements
	// spanning [Lo, Hi]. Count == Hi-Lo+1 iff the sweep is contiguous.
	Lo, Hi, Count int

	// Sectors is the number of distinct GmemSectorBytes sectors touched —
	// the GMEM objective (lower is better, contiguous = minimal).
	Sectors int

	// DistinctBanks is the number of distinct SMEM banks touched — the SMEM
	// objective (== min(Count, 32) iff conflict-free).
	DistinctBanks int
}

// Contiguous reports whether the sweep covers a contiguous element range.
func (self *SweepStats) Contiguous() bool {
	return self.Count == self.Hi-self.Lo+1
}

// BankConflictFree reports whether a warp's sweep hits pairwise distinct
// banks: DistinctBanks saturates at min(element count, bank count).
func (self *SweepStats) BankConflictFree() bool {
	maxBanks := self.Count
	if maxBanks > SmemBankCount {
		maxBanks = SmemBankCount
	}
	return self.DistinctBanks == maxBanks
}

// SweepLane evaluates the layout's byte-offset expression over the lane
// variable's full range with every other BoV variable pinned at 0.
func SweepLane(
	env Env,
	layout *Layout,
	bov []*Variable,
	lane *Variable,
	dtypeSize int,
) (SweepStats, error) {
	offset, err := StrideMap(env, layout, dtypeSize)
	if err != nil {
		return SweepStats{}, err
	}

	assignment := make(map[string]int, len(bov))
	for _, v := range bov {
		assignment[v.Name()] = 0
	}

	var out SweepStats
	seenElem := make(map[int]bool)
	seenSector := make(map[int]bool)
	seenBank := make(map[int]bool)

	first := true
	for l := 0; l < int(lane.UpperBound()); l++ {
		assignment[lane.Name()] = l
		byteOff, err := Eval(offset, assignment)
		if err != nil {
			return SweepStats{}, err
		}
		if byteOff < 0 {
			return SweepStats{}, fmt.Errorf("negative byte offset %d", byteOff)
		}
		elem := byteOff / dtypeSize
		seenElem[elem] = true
		seenSector[byteOff/GmemSectorBytes] = true
		seenBank[(byteOff/SmemBankWidthBytes)%SmemBankCount] = true
		if first || elem < out.Lo {
			out.Lo = elem
		}
		if first || elem > out.Hi {
			out.Hi = elem
		}
		first = false
	}
	out.Count = len(seenElem)
	out.Sectors = len(seenSector)
	out.DistinctBanks = len(seenBank)
	return out, nil
}
