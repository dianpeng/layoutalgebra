// Package layout is the layer-3 universal integration facade: the user picks
// an algebra by Kind, builds a layoutalgebra.Problem, and drives the pipeline
// stages (Access / GmemOptimize / SmemOptimize) on one Session, receiving
// algebra-agnostic ast.Expr artifacts only — no algebra-native types leak.
//
// Importing this package also registers both shipped algebras (f2,
// subgroupchain) in the root layoutalgebra registry, so registry consumers
// can Lookup them by name. The registry adapters carry default optimization
// windows; use New + Option for tuned windows.
//
// Typical use:
//
//	eng, _ := layout.New(layout.F2) // or layout.SubgroupChain
//	sess, _ := eng.NewSession(layoutalgebra.Problem{...})
//	gmem, _ := sess.GmemOptimize() // gmem.Offset -> translate into compiler IR
//
// The same stage methods and artifact types serve both algebras: choosing
// F₂ vs SubgroupChain is a one-token change.
package layout

import (
	"fmt"

	"github.com/dianpeng/layoutalgebra"
	"github.com/dianpeng/layoutalgebra/pkg/ast"
)

// Kind selects the layout algebra behind an Engine.
type Kind string

const (
	// F2 is the bit-level F₂ algebra: power-of-2 tiles and strides.
	F2 Kind = "f2"
	// SubgroupChain is the Z/k sub-group chain algebra: arbitrary tile
	// factorizations with CuTE-style hierarchical tuples.
	SubgroupChain Kind = "subgroupchain"
)

// Defaults follow current CUDA hardware: 128B cache lines, 128B bank
// transaction spans, bank window at byte bits [2, 7) (32 banks x 4B words).
const (
	defaultLineBytes = 128
	defaultBankBytes = 128
	defaultBankSkip  = 2
	defaultBankRange = 5
	minLineBytes     = 32
	f2SectorBits     = 5 // 32B sectors, architecture-fixed for the Segment
)

// options carries the engine-wide optimization windows.
type options struct {
	lineBytes int // GMEM coalesce line size in bytes (power of 2)
	bankBytes int // SMEM bank transaction span in bytes
	bankSkip  int // bank window start, bits past the window base
	bankRange int // bank window width in bits
}

// Option configures an Engine; see New.
type Option func(*options)

// WithLineBytes sets the GMEM coalesce line size in bytes. It feeds the F₂
// Segment (Sector 5 = 32B, Line = log2(n)) and the sub-group chain's
// GMemCoalesce.ByteSize. n must be a power of two and at least 32.
func WithLineBytes(n int) Option {
	return func(o *options) { o.lineBytes = n }
}

// WithBankBytes sets the SMEM bank transaction span in bytes (banks x word
// width). It feeds the sub-group chain's SmemCoalesce.BankBytes. n must be
// positive.
func WithBankBytes(n int) Option {
	return func(o *options) { o.bankBytes = n }
}

// WithBankWindow sets the SMEM bank analysis window: skip leading bits and
// take the next range bits (CUDA: 2, 5). skip must be >= 0 and range >= 1.
func WithBankWindow(skip, rng int) Option {
	return func(o *options) {
		o.bankSkip = skip
		o.bankRange = rng
	}
}

func defaultOptions() options {
	return options{
		lineBytes: defaultLineBytes,
		bankBytes: defaultBankBytes,
		bankSkip:  defaultBankSkip,
		bankRange: defaultBankRange,
	}
}

func validateOptions(o options) error {
	if o.lineBytes < minLineBytes || !isPow2(o.lineBytes) {
		return fmt.Errorf(
			"layout: line bytes %d is invalid, want a power of 2 >= %d",
			o.lineBytes, minLineBytes)
	}
	if o.bankBytes < 1 {
		return fmt.Errorf("layout: bank bytes %d must be positive", o.bankBytes)
	}
	if o.bankSkip < 0 {
		return fmt.Errorf("layout: bank window skip %d must be >= 0", o.bankSkip)
	}
	if o.bankRange < 1 {
		return fmt.Errorf("layout: bank window range %d must be >= 1", o.bankRange)
	}
	return nil
}

// backend is the per-algebra integration the Engine dispatches on. The
// facade holds its backend directly (no registry indirection); the separate
// root-Algebra adapters serve registry consumers.
type backend interface {
	kind() Kind
	supported(p *layoutalgebra.Problem) bool
	access(p *layoutalgebra.Problem, o options) (*Mapping, error)
	gmem(p *layoutalgebra.Problem, o options) (*Mapping, error)
	smem(p *layoutalgebra.Problem, o options) (*Mapping, error)
}

// Engine is a configured algebra instance — the abstract factory users hold.
type Engine struct {
	backend backend
	opts    options
}

// New returns an Engine for the given algebra kind with the given options
// applied on top of the CUDA defaults (128B lines, 128B bank spans, bank
// window (2, 5)). Options are validated here so misconfiguration fails at
// the boundary, not mid-pipeline.
func New(kind Kind, opts ...Option) (*Engine, error) {
	o := defaultOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	if err := validateOptions(o); err != nil {
		return nil, err
	}
	var b backend
	switch kind {
	case F2:
		b = f2Backend{}
	case SubgroupChain:
		b = sgcBackend{}
	default:
		return nil, fmt.Errorf("layout: unknown algebra kind %q", kind)
	}
	return &Engine{backend: b, opts: o}, nil
}

// Kind reports the algebra this engine drives.
func (e *Engine) Kind() Kind { return e.backend.kind() }

// NewSession validates the problem against the algebra's preconditions and
// returns a Session for it. Problems are rejected loudly at the boundary:
// Problem.Validate first, then the algebra's Supported check.
func (e *Engine) NewSession(p layoutalgebra.Problem) (*Session, error) {
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("layout: invalid problem: %w", err)
	}
	if !e.backend.supported(&p) {
		return nil, fmt.Errorf(
			"layout: algebra %q does not support the problem", e.backend.kind())
	}
	return &Session{problem: p, backend: e.backend, opts: e.opts}, nil
}

// Session is one validated problem bound to an algebra. Every stage method
// is independent: it rebuilds the native pipeline from the problem and
// lowers fresh ASTs, so stages never observe each other's side effects.
type Session struct {
	problem layoutalgebra.Problem
	backend backend
	opts    options
}

// Problem returns the problem this session was created for. Callers must
// not mutate it.
func (s *Session) Problem() *layoutalgebra.Problem { return &s.problem }

// Algebra reports the session's algebra kind.
func (s *Session) Algebra() Kind { return s.backend.kind() }

// Access runs the access-tiling stage: the mapping from the concurrency
// space into the tile coordinate space, plus its byte-offset expression.
func (s *Session) Access() (*Mapping, error) {
	return s.backend.access(&s.problem, s.opts)
}

// GmemOptimize runs the GMEM coalescing stage on a freshly built pipeline.
func (s *Session) GmemOptimize() (*Mapping, error) {
	return s.backend.gmem(&s.problem, s.opts)
}

// SmemOptimize runs the SMEM bank-conflict stage on a freshly built
// pipeline. Conservative contract: when the algebra cannot optimize the
// tiling, the native verdict travels back as an error.
func (s *Session) SmemOptimize() (*Mapping, error) {
	return s.backend.smem(&s.problem, s.opts)
}

// Mapping is the facade's stage result: algebra-agnostic artifacts only.
//
// Access is the tile-coordinate mapping AST — a Tuple with one child per
// bit, MSB-first, for F₂; a hierarchy mirroring the tile shape for the
// sub-group chain. Offset is the final byte-offset AST the compiler
// translates into its own IR.
//
// For the SMEM stage of the F₂ algebra, Access is the PRE-swizzle
// coordinate mapping while Offset is the POST-swizzle byte offset: the
// swizzle lives in byte space and is not a coordinate permutation.
type Mapping struct {
	Algebra Kind
	Access  ast.Expr
	Offset  ast.Expr
}

// --- small integer helpers shared by the backends ---

func isPow2(n int) bool { return n > 0 && n&(n-1) == 0 }

// log2Checked returns the base-2 logarithm of a power of two, or an error
// for anything else (input validation lives at every boundary).
func log2Checked(n int) (int, error) {
	if n < 1 || !isPow2(n) {
		return 0, fmt.Errorf("%d is not a power of two", n)
	}
	r := 0
	for n > 1 {
		n >>= 1
		r++
	}
	return r, nil
}

// log2Int is the unchecked variant for values already gated by Supported
// (test helpers use it on fixture problems).
func log2Int(n int) int {
	r, err := log2Checked(n)
	if err != nil {
		panic(err)
	}
	return r
}
