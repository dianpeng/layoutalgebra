package layout

import (
	"fmt"
	"strconv"

	"github.com/dianpeng/layoutalgebra"
	"github.com/dianpeng/layoutalgebra/internal/f2"
)

// --------------------------------------------------------------------------------------
//
// F₂ backend + root-registry adapter
//
// Problem -> native translation conventions:
//
//   - every tile extent and concurrency bound must be a power of two, and
//     the total bit ranks must match: Σlog2(bounds) == Σlog2(extents)
//   - bit j of variable v is named v.Name + j (j = significance, 0 = LSB)
//   - the hardware vector is assembled MSB-first following the
//     Problem.Concurrency order, each variable's bits high-index-first.
//     Put the lane variable LAST so its bits trail — the CUDA convention
//     the GMEM/SMEM stages assume
//   - f2.StrideMap and f2.SmemOptimize take the dtype size as an EXPONENT:
//     bytes 1/2/4 convert to 0/1/2
//   - dataCoord for Access/SMEM is the hardware vector itself (the identity
//     permutation, ie the "normal" tiling). Under identity tiling the SMEM
//     stage either saturates (bank window fully ranked by lane bits) or
//     fails loudly with the native no-free-degree verdict; rectangular
//     swizzle patterns stay a native-pipeline-only feature
//
// --------------------------------------------------------------------------------------

// f2Artifacts is the opaque native payload of an f2 Mapping: it travels in
// layoutalgebra.Mapping.Value and is interpreted only by this package.
type f2Artifacts struct {
	hw        f2.Vector     // hardware coordinate bit vector (MSB-first)
	coord     f2.Vector     // the stage's coordinate vector (MSB-first)
	axes      []f2.GmemAxis // logical tile axes (zero-bit axes dropped)
	laneBits  int           // bit width of the lane variable
	dtypeExp  int           // log2 of the dtype size in bytes
	shiftBits int           // byte-space shift for the adapter StrideMap
}

// f2Offset is the opaque native payload of an f2 Offset: the final
// byte-space bit vector (MSB-first).
type f2Offset struct {
	vec f2.Vector
}

// f2Backend implements the facade's backend interface for the F₂ algebra.
type f2Backend struct{}

func (f2Backend) kind() Kind { return F2 }

func (f2Backend) supported(p *layoutalgebra.Problem) bool { return f2Supported(p) }

func (f2Backend) access(p *layoutalgebra.Problem, _ options) (*Mapping, error) {
	art, err := f2AccessArtifacts(p)
	if err != nil {
		return nil, err
	}
	return f2FacadeMapping(art, art.coord)
}

func (f2Backend) gmem(p *layoutalgebra.Problem, o options) (*Mapping, error) {
	art, err := f2GmemArtifacts(p, o)
	if err != nil {
		return nil, err
	}
	return f2FacadeMapping(art, art.coord)
}

func (f2Backend) smem(p *layoutalgebra.Problem, o options) (*Mapping, error) {
	art, logical, err := f2SmemStage(p, o)
	if err != nil {
		return nil, err
	}
	// D7: Access is the PRE-swizzle coordinate mapping; Offset is the
	// POST-swizzle byte offset (coord carries it, shiftBits == 0).
	return f2FacadeMapping(art, logical)
}

// f2FacadeMapping lowers one completed stage into the facade Mapping:
// coordAST is the tile-coordinate mapping, and the offset comes from the
// adapter stride map over the stage artifacts.
func f2FacadeMapping(art f2Artifacts, coordASTSource f2.Vector) (*Mapping, error) {
	offsetVec, err := f2Stride(art)
	if err != nil {
		return nil, err
	}
	accessAST, err := lowerF2Vector(coordASTSource)
	if err != nil {
		return nil, err
	}
	offsetAST, err := lowerF2Offset(offsetVec)
	if err != nil {
		return nil, err
	}
	return &Mapping{Algebra: F2, Access: accessAST, Offset: offsetAST}, nil
}

// f2Stride is the adapter stride map: shifts the stage's coordinate vector
// into byte space. The dummy shape slice of matching length is the
// established native pipeline idiom.
func f2Stride(art f2Artifacts) (f2.Vector, error) {
	return f2.StrideMap(make([]int, art.coord.Size()), art.coord, art.shiftBits)
}

// f2Supported reports whether the problem fits the F₂ algebra:
// power-of-2 extents/bounds and equal total bit ranks.
func f2Supported(p *layoutalgebra.Problem) bool {
	hwBits, tileBits := 0, 0
	for _, v := range p.Concurrency {
		b, err := log2Checked(v.Bound)
		if err != nil {
			return false
		}
		hwBits += b
	}
	for _, extent := range p.Tile.Shape {
		b, err := log2Checked(extent)
		if err != nil {
			return false
		}
		tileBits += b
	}
	return hwBits == tileBits
}

// f2Prepare builds the shared native setup from a (Supported) problem: the
// hardware vector, the logical tile axes, the lane bit width and the dtype
// exponent.
func f2Prepare(p *layoutalgebra.Problem) (f2.Vector, []f2.GmemAxis, int, int, error) {
	elems := []f2.Expr{}
	hwBits := 0
	for _, v := range p.Concurrency {
		bits, err := log2Checked(v.Bound)
		if err != nil {
			return f2.Vector{}, nil, 0, 0,
				fmt.Errorf("f2: variable %q bound %d: %w", v.Name, v.Bound, err)
		}
		hwBits += bits
		for j := bits - 1; j >= 0; j-- {
			elems = append(elems, f2.NewVariable(v.Name+strconv.Itoa(j)))
		}
	}

	axes := []f2.GmemAxis{}
	tileBits := 0
	for i, extent := range p.Tile.Shape {
		bits, err := log2Checked(extent)
		if err != nil {
			return f2.Vector{}, nil, 0, 0,
				fmt.Errorf("f2: tile shape[%d] = %d: %w", i, extent, err)
		}
		tileBits += bits
		if bits > 0 { // GmemCoalescingMap rejects zero-bit axes
			axes = append(axes, f2.GmemAxis{Name: fmt.Sprintf("d%d", i), Bits: bits})
		}
	}
	if hwBits != tileBits {
		return f2.Vector{}, nil, 0, 0, fmt.Errorf(
			"f2: hardware bit rank %d does not match tile bit rank %d", hwBits, tileBits)
	}

	lane := p.Variable(p.Lane)
	if lane == nil {
		return f2.Vector{}, nil, 0, 0,
			fmt.Errorf("f2: lane variable %q is not in the concurrency space", p.Lane)
	}
	laneBits, err := log2Checked(lane.Bound)
	if err != nil {
		return f2.Vector{}, nil, 0, 0,
			fmt.Errorf("f2: lane bound %d: %w", lane.Bound, err)
	}
	dtypeExp, err := log2Checked(p.Tile.DTypeSize)
	if err != nil {
		return f2.Vector{}, nil, 0, 0,
			fmt.Errorf("f2: dtype size %d: %w", p.Tile.DTypeSize, err)
	}
	return f2.NewVector(elems), axes, laneBits, dtypeExp, nil
}

// f2AccessArtifacts runs the access stage: the identity tiling
// (dataCoord == hw), byte-shifted by the dtype exponent.
func f2AccessArtifacts(p *layoutalgebra.Problem) (f2Artifacts, error) {
	hw, axes, laneBits, dtypeExp, err := f2Prepare(p)
	if err != nil {
		return f2Artifacts{}, err
	}
	coord, err := f2.MapAccess(hw, hw)
	if err != nil {
		return f2Artifacts{}, fmt.Errorf("f2 access: %w", err)
	}
	return f2Artifacts{
		hw: hw, coord: coord, axes: axes,
		laneBits: laneBits, dtypeExp: dtypeExp, shiftBits: dtypeExp,
	}, nil
}

// f2GmemArtifacts runs the GMEM coalescing stage: the certified plan's
// matrix M applied to the hardware vector, byte-shifted by the dtype
// exponent.
func f2GmemArtifacts(p *layoutalgebra.Problem, o options) (f2Artifacts, error) {
	hw, axes, laneBits, dtypeExp, err := f2Prepare(p)
	if err != nil {
		return f2Artifacts{}, err
	}
	seg, err := f2SegmentFor(o.lineBytes)
	if err != nil {
		return f2Artifacts{}, err
	}
	res, err := f2.GmemOptimizeWith(hw, axes, laneBits, dtypeExp, seg)
	if err != nil {
		return f2Artifacts{}, fmt.Errorf("f2 gmem: %w", err)
	}
	return f2Artifacts{
		hw: hw, coord: res.Plan.Matrix.Apply(hw), axes: axes,
		laneBits: laneBits, dtypeExp: dtypeExp, shiftBits: dtypeExp,
	}, nil
}

// f2SmemStage runs the SMEM bank-conflict stage: identity tiling shifted
// into byte space, then XOR-swizzled. coord carries the POST-swizzle byte
// vector (shiftBits 0); logical is the PRE-swizzle coordinate mapping the
// facade reports as Access.
func f2SmemStage(p *layoutalgebra.Problem, o options) (f2Artifacts, f2.Vector, error) {
	hw, axes, laneBits, dtypeExp, err := f2Prepare(p)
	if err != nil {
		return f2Artifacts{}, f2.Vector{}, err
	}
	logical, err := f2.MapAccess(hw, hw)
	if err != nil {
		return f2Artifacts{}, f2.Vector{}, fmt.Errorf("f2 smem: %w", err)
	}
	lane := p.Variable(p.Lane)
	bank := f2.Bank{Skip: o.bankSkip, Range: o.bankRange, VariablePrefix: lane.Name}
	res, err := f2.SmemOptimize(hw, hw, dtypeExp, bank)
	if err != nil {
		return f2Artifacts{}, f2.Vector{}, fmt.Errorf("f2 smem: %w", err)
	}
	return f2Artifacts{
		hw: hw, coord: res.Offset, axes: axes,
		laneBits: laneBits, dtypeExp: dtypeExp, shiftBits: 0,
	}, logical, nil
}

// f2SegmentFor converts the line-bytes option into the native Segment:
// 32B sectors (bit 5) and log2(n) byte lines.
func f2SegmentFor(lineBytes int) (f2.Segment, error) {
	line, err := log2Checked(lineBytes)
	if err != nil {
		return f2.Segment{}, fmt.Errorf("f2 segment: %w", err)
	}
	if line <= f2SectorBits {
		return f2.Segment{}, fmt.Errorf(
			"f2 segment: line bytes %d (line bit %d) must exceed the 32B sector",
			lineBytes, line)
	}
	return f2.Segment{Sector: f2SectorBits, Line: line}, nil
}

// --------------------------------------------------------------------------------------
// Root-registry adapter
// --------------------------------------------------------------------------------------

// algebraF2 adapts the F₂ algebra to the root layoutalgebra.Algebra
// interface. Registry consumers get the DEFAULT optimization windows; use
// the Engine facade (New + Options) for tuned windows. Native artifacts
// travel opaquely in Mapping/Offset Value, exactly as doc.go envisions.
type algebraF2 struct{}

func (algebraF2) Name() string                            { return string(F2) }
func (algebraF2) Supported(p *layoutalgebra.Problem) bool { return f2Supported(p) }
func (algebraF2) Access(p *layoutalgebra.Problem) (layoutalgebra.Mapping, error) {
	art, err := f2AccessArtifacts(p)
	if err != nil {
		return layoutalgebra.Mapping{}, err
	}
	return layoutalgebra.Mapping{Algebra: string(F2), Value: art}, nil
}

func (algebraF2) StrideMap(m layoutalgebra.Mapping, _ *layoutalgebra.Problem) (layoutalgebra.Offset, error) {
	if err := checkAlgebraName(m.Algebra, F2); err != nil {
		return layoutalgebra.Offset{}, err
	}
	art, err := asF2Artifacts(m.Value)
	if err != nil {
		return layoutalgebra.Offset{}, err
	}
	vec, err := f2Stride(art)
	if err != nil {
		return layoutalgebra.Offset{}, err
	}
	return layoutalgebra.Offset{Algebra: string(F2), Value: f2Offset{vec: vec}}, nil
}

func (algebraF2) GmemOptimize(m layoutalgebra.Mapping, _ *layoutalgebra.Problem) (layoutalgebra.Mapping, error) {
	if err := checkAlgebraName(m.Algebra, F2); err != nil {
		return layoutalgebra.Mapping{}, err
	}
	art, err := asF2Artifacts(m.Value)
	if err != nil {
		return layoutalgebra.Mapping{}, err
	}
	seg, err := f2SegmentFor(defaultOptions().lineBytes)
	if err != nil {
		return layoutalgebra.Mapping{}, err
	}
	res, err := f2.GmemOptimizeWith(art.hw, art.axes, art.laneBits, art.dtypeExp, seg)
	if err != nil {
		return layoutalgebra.Mapping{}, fmt.Errorf("f2 gmem: %w", err)
	}
	return layoutalgebra.Mapping{Algebra: string(F2), Value: f2Artifacts{
		hw: art.hw, coord: res.Plan.Matrix.Apply(art.hw), axes: art.axes,
		laneBits: art.laneBits, dtypeExp: art.dtypeExp, shiftBits: art.dtypeExp,
	}}, nil
}

func (algebraF2) SmemOptimize(m layoutalgebra.Mapping, p *layoutalgebra.Problem) (layoutalgebra.Mapping, error) {
	if err := checkAlgebraName(m.Algebra, F2); err != nil {
		return layoutalgebra.Mapping{}, err
	}
	art, err := asF2Artifacts(m.Value)
	if err != nil {
		return layoutalgebra.Mapping{}, err
	}
	lane := p.Variable(p.Lane)
	if lane == nil {
		return layoutalgebra.Mapping{}, fmt.Errorf(
			"f2 smem: lane variable %q is not in the concurrency space", p.Lane)
	}
	o := defaultOptions()
	bank := f2.Bank{Skip: o.bankSkip, Range: o.bankRange, VariablePrefix: lane.Name}
	res, err := f2.SmemOptimize(art.hw, art.hw, art.dtypeExp, bank)
	if err != nil {
		return layoutalgebra.Mapping{}, fmt.Errorf("f2 smem: %w", err)
	}
	return layoutalgebra.Mapping{Algebra: string(F2), Value: f2Artifacts{
		hw: art.hw, coord: res.Offset, axes: art.axes,
		laneBits: art.laneBits, dtypeExp: art.dtypeExp, shiftBits: 0,
	}}, nil
}

func init() {
	layoutalgebra.Register(algebraF2{})
}

// --- payload type guards (trust boundary: never panic on unknown Value) ---

func checkAlgebraName(got string, want Kind) error {
	if got != string(want) {
		return fmt.Errorf(
			"layout: artifact algebra %q does not match the %q adapter", got, want)
	}
	return nil
}

func asF2Artifacts(v any) (f2Artifacts, error) {
	art, ok := v.(f2Artifacts)
	if !ok {
		return f2Artifacts{}, fmt.Errorf(
			"layout: f2 Mapping carries %T, want the f2 stage artifacts", v)
	}
	return art, nil
}

func asF2Offset(v any) (f2Offset, error) {
	off, ok := v.(f2Offset)
	if !ok {
		return f2Offset{}, fmt.Errorf(
			"layout: f2 Offset carries %T, want the f2 offset artifacts", v)
	}
	return off, nil
}

// ensure the adapters satisfy the root interface
var _ layoutalgebra.Algebra = algebraF2{}
