package layoutalgebra

import "fmt"

// Tile describes the tile under analysis: its per-dimension extents and the
// element size in bytes. Individual algebras impose their own preconditions
// on the shape (the F₂ algebra requires power-of-2 extents; the sub-group
// chain algebra accepts arbitrary integer factorizations) — Supported
// reports them.
type Tile struct {
	Shape     []int
	DTypeSize int
}

// Variable is one named hardware concurrency variable together with its
// bound; valid values are [0, Bound).
type Variable struct {
	// Family carries semantic meaning for the upper layer, eg "warp", "lane".
	Family string
	// Name is unique within a Problem and identifies the variable in results.
	Name  string
	Bound int
}

// Problem is the algebra-agnostic problem description handed to every Algebra
// implementation. Implementations must not mutate it.
type Problem struct {
	Tile        Tile
	Concurrency []Variable

	// Lane is the Name of the per-warp induction variable within Concurrency
	// (the variable a warp's threads differ in, eg threadIdx % 32). GMEM/SMEM
	// optimization stages score the mapping against its sweep.
	Lane string
}

// Variable returns the concurrency variable with the given name, or nil.
func (self *Problem) Variable(name string) *Variable {
	for i := range self.Concurrency {
		if self.Concurrency[i].Name == name {
			return &self.Concurrency[i]
		}
	}
	return nil
}

// Validate checks the problem description is well-formed before it is handed
// to an algebra.
func (self *Problem) Validate() error {
	switch self.Tile.DTypeSize {
	case 1, 2, 4:
	default:
		return fmt.Errorf("invalid dtype size %d, want 1, 2 or 4", self.Tile.DTypeSize)
	}
	if len(self.Tile.Shape) == 0 {
		return fmt.Errorf("empty tile shape")
	}
	for idx, extent := range self.Tile.Shape {
		if extent < 1 {
			return fmt.Errorf("tile shape[%d] = %d, must be >= 1", idx, extent)
		}
	}
	if len(self.Concurrency) == 0 {
		return fmt.Errorf("empty concurrency space")
	}
	seen := make(map[string]bool, len(self.Concurrency))
	for _, v := range self.Concurrency {
		if v.Bound < 1 {
			return fmt.Errorf("variable %q has bound %d, must be >= 1", v.Name, v.Bound)
		}
		if seen[v.Name] {
			return fmt.Errorf("duplicate variable name %q", v.Name)
		}
		seen[v.Name] = true
	}
	if self.Variable(self.Lane) == nil {
		return fmt.Errorf("lane variable %q is not in the concurrency space", self.Lane)
	}
	return nil
}

// Mapping is the opaque result of the access stage: the algebra-specific
// mapping from the concurrency space to the tile coordinate space. Value
// holds the implementation's native representation; only the producing
// algebra interprets it (today: its own pipeline stages; later: its
// code-generation backend).
type Mapping struct {
	// Algebra is the Name of the producing implementation.
	Algebra string
	Value   any
}

// Offset is the opaque byte-oriented memory offset expression produced by the
// stride-mapping stage, scaled by the tile's dtype size. Same opacity rules
// as Mapping.
type Offset struct {
	Algebra string
	Value   any
}

// Algebra is the universal interface implemented by every layout algebra.
// Upper layers integrate against this interface only; implementations are
// looked up by name (or chosen by capability) in the registry.
//
// The pipeline stages mirror the shared vocabulary of every layout algebra:
//
//	Access        concurrency space |-> tile coordinate space
//	StrideMap     tile coordinates  |-> byte-oriented memory offset
//	GmemOptimize  rewrite the access mapping for GMEM coalescing
//	SmemOptimize  rewrite the access mapping for SMEM bank-conflict avoidance
//
// Notes
//
//   - The contract is deliberately thin: Mapping/Offset are opaque, so
//     algebra-specific plan reports, scores, and verdicts travel inside Value
//     until the code-generation layer pins down a richer interchange form.
//   - Stages that an algebra cannot perform must return an error (conservative
//     contract), never a silently unoptimized result.
//   - The interface will evolve as the second full adapter and the
//     code-generation layer land; do not bake caller-side assumptions about
//     Value's dynamic type beyond the producing algebra's own package.
type Algebra interface {
	// Name identifies the implementation, eg "f2", "subgroupchain". Unique
	// across the registry.
	Name() string

	// Supported reports whether this algebra can handle the given problem,
	// eg power-of-2-only algebras reject arbitrary tile extents. Validate
	// has already succeeded when Supported is called.
	Supported(problem *Problem) bool

	// Access maps the concurrency space into the tile coordinate space.
	Access(problem *Problem) (Mapping, error)

	// StrideMap maps a coordinate Mapping into a byte-oriented Offset.
	StrideMap(m Mapping, problem *Problem) (Offset, error)

	// GmemOptimize rewrites the access Mapping for global-memory coalescing
	// (eg re-searching the variable ordering), returning the improved
	// Mapping.
	GmemOptimize(m Mapping, problem *Problem) (Mapping, error)

	// SmemOptimize rewrites the access Mapping for shared-memory
	// bank-conflict avoidance, returning the improved Mapping. An algebra
	// without a transform yet returns an error carrying its analysis verdict.
	SmemOptimize(m Mapping, problem *Problem) (Mapping, error)
}

// --------------------------------------------------------------------------------------
// Registry
//
// Implementations register themselves at init time; upper layers look them up
// afterwards. Registration is not goroutine-safe by design — it happens during
// package initialization, before any reader exists.
// --------------------------------------------------------------------------------------

var registry = map[string]Algebra{}

// Register adds an algebra to the registry. It panics if the name is empty or
// already taken — registration errors are programming errors, not runtime
// conditions.
func Register(a Algebra) {
	if a == nil {
		panic("layoutalgebra: Register(nil)")
	}
	name := a.Name()
	if name == "" {
		panic("layoutalgebra: Register with empty Name")
	}
	if _, ok := registry[name]; ok {
		panic(fmt.Sprintf("layoutalgebra: duplicate algebra name %q", name))
	}
	registry[name] = a
}

// Lookup returns the algebra registered under name.
func Lookup(name string) (Algebra, bool) {
	a, ok := registry[name]
	return a, ok
}

// Names returns the registered algebra names, sorted for determinism.
func Names() []string {
	out := make([]string, 0, len(registry))
	for name := range registry {
		out = append(out, name)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
