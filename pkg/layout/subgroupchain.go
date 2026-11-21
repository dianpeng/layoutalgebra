package layout

import (
	"fmt"

	"github.com/dianpeng/layoutalgebra"
	"github.com/dianpeng/layoutalgebra/internal/subgroupchain"
)

// --------------------------------------------------------------------------------------
//
// Sub-group chain backend + root-registry adapter
//
// Problem -> native translation conventions:
//
//   - the BoV is Problem.Concurrency in the GIVEN order. Order is semantic
//     (major -> minor significance); put the lane variable LAST so its
//     freedom trails into the low-weight tail — the position the
//     GMEM/SMEM passes score against
//   - the tile is a flat NewConstHTuple of the shape extents; nested
//     (hierarchical) shapes stay a native-pipeline-only feature
//   - native StrideMap takes the dtype size in BYTES (1/2/4) — no exponent
//     conversion, unlike the F₂ convention
//   - the native GMem/Smem passes mutate the layout in place, so every
//     adapter entry point deep-copies the Coordinate HTuple first; the
//     Shape is constants-only and safely shared (immutable pattern)
//   - native errors propagate unchanged (conservative contract): a pass
//     that cannot optimize reports its verdict, never a silent no-op
//
// --------------------------------------------------------------------------------------

// sgcArtifacts is the opaque native payload of a subgroupchain Mapping: it
// travels in layoutalgebra.Mapping.Value and is interpreted only here.
type sgcArtifacts struct {
	env    *subgroupchain.Env
	bov    []*subgroupchain.Variable
	layout *subgroupchain.Layout
	lane   int // index of the lane variable within bov
}

// sgcOffset is the opaque native payload of a subgroupchain Offset: the
// native byte-offset expression.
type sgcOffset struct {
	expr subgroupchain.Expr
}

// sgcBackend implements the facade's backend interface for the sub-group
// chain algebra.
type sgcBackend struct{}

func (sgcBackend) kind() Kind { return SubgroupChain }

func (sgcBackend) supported(p *layoutalgebra.Problem) bool { return sgcSupported(p) }

func (sgcBackend) access(p *layoutalgebra.Problem, _ options) (*Mapping, error) {
	art, err := sgcAccessArtifacts(p)
	if err != nil {
		return nil, err
	}
	return sgcFacadeMapping(art, p)
}

func (sgcBackend) gmem(p *layoutalgebra.Problem, o options) (*Mapping, error) {
	art, err := sgcGmemArtifacts(p, o)
	if err != nil {
		return nil, err
	}
	return sgcFacadeMapping(art, p)
}

func (sgcBackend) smem(p *layoutalgebra.Problem, o options) (*Mapping, error) {
	art, err := sgcSmemArtifacts(p, o)
	if err != nil {
		return nil, err
	}
	return sgcFacadeMapping(art, p)
}

// sgcFacadeMapping lowers one completed stage into the facade Mapping.
func sgcFacadeMapping(art sgcArtifacts, p *layoutalgebra.Problem) (*Mapping, error) {
	accessAST, err := lowerHTuple(art.layout.Coordinate)
	if err != nil {
		return nil, err
	}
	offsetExpr, err := subgroupchain.StrideMap(*art.env, art.layout, p.Tile.DTypeSize)
	if err != nil {
		return nil, fmt.Errorf("subgroupchain stride map: %w", err)
	}
	offsetAST, err := lowerSGCExpr(offsetExpr)
	if err != nil {
		return nil, err
	}
	return &Mapping{Algebra: SubgroupChain, Access: accessAST, Offset: offsetAST}, nil
}

// sgcSupported reports whether the problem fits the sub-group chain
// algebra: the native soundness precondition Π(bounds) == Π(extents).
// Arbitrary (non-power-of-2) factorizations are accepted.
func sgcSupported(p *layoutalgebra.Problem) bool {
	bounds := 1
	for _, v := range p.Concurrency {
		if v.Bound < 1 {
			return false
		}
		bounds *= v.Bound
	}
	extents := 1
	for _, e := range p.Tile.Shape {
		if e < 1 {
			return false
		}
		extents *= e
	}
	return bounds == extents
}

// sgcAccessArtifacts builds the native access mapping from the problem.
func sgcAccessArtifacts(p *layoutalgebra.Problem) (sgcArtifacts, error) {
	env := subgroupchain.NewEnv()
	bov := make([]*subgroupchain.Variable, 0, len(p.Concurrency))
	lane := -1
	for i, v := range p.Concurrency {
		bov = append(bov, env.Factory.Variable(v.Family, v.Name, v.Bound))
		if v.Name == p.Lane {
			lane = i
		}
	}
	if lane < 0 {
		// unreachable after Problem.Validate, kept as a boundary guard
		return sgcArtifacts{}, fmt.Errorf(
			"subgroupchain: lane variable %q is not in the concurrency space", p.Lane)
	}

	shape := make([]subgroupchain.Value, 0, len(p.Tile.Shape))
	for _, extent := range p.Tile.Shape {
		shape = append(shape, subgroupchain.Value(extent))
	}
	tile := subgroupchain.NewConstHTuple(shape)

	ht, err := subgroupchain.Access(*env, bov, tile)
	if err != nil {
		return sgcArtifacts{}, fmt.Errorf("subgroupchain access: %w", err)
	}
	return sgcArtifacts{
		env:    env,
		bov:    bov,
		layout: &subgroupchain.Layout{Coordinate: ht, Shape: tile},
		lane:   lane,
	}, nil
}

// cloneLayout deep-copies the coordinate hierarchy (the native passes
// mutate in place); the constants-only Shape is shared.
func (a sgcArtifacts) cloneLayout() *subgroupchain.Layout {
	return &subgroupchain.Layout{
		Coordinate: a.layout.Coordinate.Clone(),
		Shape:      a.layout.Shape,
	}
}

// sgcGmemArtifacts runs the GMEM coalescing pass on a deep copy.
func sgcGmemArtifacts(p *layoutalgebra.Problem, o options) (sgcArtifacts, error) {
	art, err := sgcAccessArtifacts(p)
	if err != nil {
		return sgcArtifacts{}, err
	}
	clone := art.cloneLayout()
	cfg := subgroupchain.GMemCoalesce{
		ByteSize:       o.lineBytes,
		VariableFamily: art.bov[art.lane].Family(),
	}
	if err := subgroupchain.GMem(*art.env, cfg, p.Tile.DTypeSize, clone); err != nil {
		return sgcArtifacts{}, fmt.Errorf("subgroupchain gmem: %w", err)
	}
	return sgcArtifacts{env: art.env, bov: art.bov, layout: clone, lane: art.lane}, nil
}

// sgcSmemArtifacts runs the SMEM bank-conflict pass on a deep copy.
func sgcSmemArtifacts(p *layoutalgebra.Problem, o options) (sgcArtifacts, error) {
	art, err := sgcAccessArtifacts(p)
	if err != nil {
		return sgcArtifacts{}, err
	}
	clone := art.cloneLayout()
	cfg := subgroupchain.SmemCoalesce{
		VariableFamily: art.bov[art.lane].Family(),
		BankBytes:      o.bankBytes,
	}
	if err := subgroupchain.Smem(*art.env, cfg, art.bov, p.Tile.DTypeSize, clone); err != nil {
		return sgcArtifacts{}, fmt.Errorf("subgroupchain smem: %w", err)
	}
	return sgcArtifacts{env: art.env, bov: art.bov, layout: clone, lane: art.lane}, nil
}

// --------------------------------------------------------------------------------------
// Root-registry adapter
// --------------------------------------------------------------------------------------

// algebraSubgroupChain adapts the sub-group chain algebra to the root
// layoutalgebra.Algebra interface. Registry consumers get the DEFAULT
// optimization windows; use the Engine facade for tuned windows.
type algebraSubgroupChain struct{}

func (algebraSubgroupChain) Name() string                            { return string(SubgroupChain) }
func (algebraSubgroupChain) Supported(p *layoutalgebra.Problem) bool { return sgcSupported(p) }

func (algebraSubgroupChain) Access(p *layoutalgebra.Problem) (layoutalgebra.Mapping, error) {
	art, err := sgcAccessArtifacts(p)
	if err != nil {
		return layoutalgebra.Mapping{}, err
	}
	return layoutalgebra.Mapping{Algebra: string(SubgroupChain), Value: art}, nil
}

func (algebraSubgroupChain) StrideMap(m layoutalgebra.Mapping, p *layoutalgebra.Problem) (layoutalgebra.Offset, error) {
	if err := checkAlgebraName(m.Algebra, SubgroupChain); err != nil {
		return layoutalgebra.Offset{}, err
	}
	art, err := asSgcArtifacts(m.Value)
	if err != nil {
		return layoutalgebra.Offset{}, err
	}
	// native StrideMap takes the dtype size in BYTES here
	expr, err := subgroupchain.StrideMap(*art.env, art.layout, p.Tile.DTypeSize)
	if err != nil {
		return layoutalgebra.Offset{}, fmt.Errorf("subgroupchain stride map: %w", err)
	}
	return layoutalgebra.Offset{Algebra: string(SubgroupChain), Value: sgcOffset{expr: expr}}, nil
}

func (algebraSubgroupChain) GmemOptimize(m layoutalgebra.Mapping, p *layoutalgebra.Problem) (layoutalgebra.Mapping, error) {
	if err := checkAlgebraName(m.Algebra, SubgroupChain); err != nil {
		return layoutalgebra.Mapping{}, err
	}
	art, err := asSgcArtifacts(m.Value)
	if err != nil {
		return layoutalgebra.Mapping{}, err
	}
	return sgcAdapterOptimize(art, p, defaultOptions())
}

func (algebraSubgroupChain) SmemOptimize(m layoutalgebra.Mapping, p *layoutalgebra.Problem) (layoutalgebra.Mapping, error) {
	if err := checkAlgebraName(m.Algebra, SubgroupChain); err != nil {
		return layoutalgebra.Mapping{}, err
	}
	art, err := asSgcArtifacts(m.Value)
	if err != nil {
		return layoutalgebra.Mapping{}, err
	}
	return sgcAdapterSmem(art, p, defaultOptions())
}

// sgcAdapterOptimize optimizes the artifacts' own layout (deep-copied) with
// the GMEM pass and wraps the result as a fresh Mapping.
func sgcAdapterOptimize(art sgcArtifacts, p *layoutalgebra.Problem, o options) (layoutalgebra.Mapping, error) {
	clone := art.cloneLayout()
	cfg := subgroupchain.GMemCoalesce{
		ByteSize:       o.lineBytes,
		VariableFamily: art.bov[art.lane].Family(),
	}
	if err := subgroupchain.GMem(*art.env, cfg, p.Tile.DTypeSize, clone); err != nil {
		return layoutalgebra.Mapping{}, fmt.Errorf("subgroupchain gmem: %w", err)
	}
	return layoutalgebra.Mapping{Algebra: string(SubgroupChain), Value: sgcArtifacts{
		env: art.env, bov: art.bov, layout: clone, lane: art.lane,
	}}, nil
}

// sgcAdapterSmem optimizes the artifacts' own layout (deep-copied) with the
// SMEM pass and wraps the result as a fresh Mapping.
func sgcAdapterSmem(art sgcArtifacts, p *layoutalgebra.Problem, o options) (layoutalgebra.Mapping, error) {
	clone := art.cloneLayout()
	cfg := subgroupchain.SmemCoalesce{
		VariableFamily: art.bov[art.lane].Family(),
		BankBytes:      o.bankBytes,
	}
	if err := subgroupchain.Smem(*art.env, cfg, art.bov, p.Tile.DTypeSize, clone); err != nil {
		return layoutalgebra.Mapping{}, fmt.Errorf("subgroupchain smem: %w", err)
	}
	return layoutalgebra.Mapping{Algebra: string(SubgroupChain), Value: sgcArtifacts{
		env: art.env, bov: art.bov, layout: clone, lane: art.lane,
	}}, nil
}

func init() {
	layoutalgebra.Register(algebraSubgroupChain{})
}

// --- payload type guards (trust boundary: never panic on unknown Value) ---

func asSgcArtifacts(v any) (sgcArtifacts, error) {
	art, ok := v.(sgcArtifacts)
	if !ok {
		return sgcArtifacts{}, fmt.Errorf(
			"layout: subgroupchain Mapping carries %T, want the sgc stage artifacts", v)
	}
	return art, nil
}

func asSgcOffset(v any) (sgcOffset, error) {
	off, ok := v.(sgcOffset)
	if !ok {
		return sgcOffset{}, fmt.Errorf(
			"layout: subgroupchain Offset carries %T, want the sgc offset artifacts", v)
	}
	return off, nil
}

// ensure the adapter satisfies the root interface
var _ layoutalgebra.Algebra = algebraSubgroupChain{}
