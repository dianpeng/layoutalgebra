package subgroupchain

import (
	"strings"
	"testing"
)

// --------------------------------------------------------------------------------------
// framework components
// --------------------------------------------------------------------------------------

func TestEvalProduction(t *testing.T) {
	env := *NewEnv()
	f := env.Factory
	v := f.Variable("t", "t", 32)
	dv := f.Dedicate(v)
	AnalyzeRange(dv)

	// coordinate + stride evaluation
	digits, err := f.Factorize(dv, []int{8, 4})
	if err != nil {
		t.Fatalf("Factorize failed: %v", err)
	}
	for x := 0; x < 32; x++ {
		got, err := Eval(digits[0], map[string]int{"t": x})
		if err != nil || got != x/4 {
			t.Fatalf("x=%d: Eval = (%d, %v), want %d", x, got, err, x/4)
		}
	}

	// error paths
	if _, err := Eval(v, nil); err == nil {
		t.Error("Eval with missing assignment: expected error")
	}
	if _, err := Eval(f.Binary(Div, f.Const(1), f.Const(0)), nil); err == nil {
		t.Error("Eval div by zero: expected error")
	}
	if _, err := Eval(f.Binary(Mod, f.Const(1), f.Const(0)), nil); err == nil {
		t.Error("Eval mod by zero: expected error")
	}
}

func TestSweepStats(t *testing.T) {
	env := *NewEnv()
	f := env.Factory
	lane := f.Variable("lane", "lane", 32)
	warp := f.Variable("warp", "warp", 8)
	tile := NewConstHTuple([]Value{8, 32})

	coord, err := Access(env, []*Variable{lane, warp}, tile)
	if err != nil {
		t.Fatalf("Access failed: %v", err)
	}

	// pre-GMem: strided sweep (32 sectors, not contiguous)
	pre, err := SweepLane(env, &Layout{Coordinate: coord, Shape: tile},
		[]*Variable{lane, warp}, lane, 4)
	if err != nil {
		t.Fatal(err)
	}
	if pre.Sectors != 32 || pre.Contiguous() {
		t.Fatalf("pre sweep = %+v, want 32 strided sectors", pre)
	}

	layout := &Layout{Coordinate: coord, Shape: tile}
	if err := GMem(env, GMemCoalesce{ByteSize: 128, VariableFamily: "lane"}, 4, layout); err != nil {
		t.Fatal(err)
	}
	post, err := SweepLane(env, layout, []*Variable{lane, warp}, lane, 4)
	if err != nil {
		t.Fatal(err)
	}
	if !post.Contiguous() || post.Sectors != 4 {
		t.Fatalf("post sweep = %+v, want contiguous 4 sectors", post)
	}
	if !post.BankConflictFree() {
		t.Fatalf("contiguous 128B sweep should be bank conflict-free: %+v", post)
	}
}

// --------------------------------------------------------------------------------------
// SMEM pass scenarios
// --------------------------------------------------------------------------------------

// Conflict-free input: the warp sweep already hits 32 distinct banks — the
// fast path must no-op (layout byte-identical).
func TestSmemFastPathNoOp(t *testing.T) {
	env := *NewEnv()
	f := env.Factory
	lane := f.Variable("lane", "lane", 32)
	warp := f.Variable("warp", "warp", 8)
	tile := NewConstHTuple([]Value{8, 32})

	coord, err := Access(env, []*Variable{warp, lane}, tile) // lane-only last dim
	if err != nil {
		t.Fatalf("Access failed: %v", err)
	}
	layout := &Layout{Coordinate: coord, Shape: tile}
	before := elementOffsets(t, env, layout, []*Variable{warp, lane}, 4)

	if err := Smem(env, SmemCoalesce{VariableFamily: "lane"},
		[]*Variable{warp, lane}, 4, layout); err != nil {
		t.Fatalf("Smem failed: %v", err)
	}
	after := elementOffsets(t, env, layout, []*Variable{warp, lane}, 4)
	if len(before) != len(after) {
		t.Fatal("conflict-free Smem changed the mapping")
	}
	for e := range before {
		if _, ok := after[e]; !ok {
			t.Fatalf("no-op Smem dropped element %d", e)
		}
	}
}

// Conflicted input without a swizzle hook: the universal coverage draw is
// the fallback and must end conflict-free.
func TestSmemCoverageFallback(t *testing.T) {
	env := *NewEnv()
	f := env.Factory
	lane := f.Variable("lane", "lane", 32)
	warp := f.Variable("warp", "warp", 8)
	tile := NewConstHTuple([]Value{8, 32})

	coord, err := Access(env, []*Variable{lane, warp}, tile)
	if err != nil {
		t.Fatalf("Access failed: %v", err)
	}
	layout := &Layout{Coordinate: coord, Shape: tile}

	ok, err := SmemBankPredicate(env, layout, []*Variable{lane, warp}, lane, 4)
	if err != nil || ok {
		t.Fatalf("pre-condition: expected conflicts, got ok=%v err=%v", ok, err)
	}

	if err := Smem(env, SmemCoalesce{VariableFamily: "lane"},
		[]*Variable{lane, warp}, 4, layout); err != nil {
		t.Fatalf("Smem failed: %v", err)
	}

	assertBijective(t, env, layout, []*Variable{lane, warp}, 4)
	ok, err = SmemBankPredicate(env, layout, []*Variable{lane, warp}, lane, 4)
	if err != nil || !ok {
		t.Fatalf("post Smem still conflicted: ok=%v err=%v", ok, err)
	}
}

// The swizzle hook is invoked before the coverage fallback, and a handled
// hook result is trusted (it owns its verification).
func TestSmemSwizzleHook(t *testing.T) {
	env := *NewEnv()
	f := env.Factory
	lane := f.Variable("lane", "lane", 32)
	warp := f.Variable("warp", "warp", 8)
	tile := NewConstHTuple([]Value{8, 32})

	coord, err := Access(env, []*Variable{lane, warp}, tile)
	if err != nil {
		t.Fatalf("Access failed: %v", err)
	}
	layout := &Layout{Coordinate: coord, Shape: tile}
	before := elementOffsets(t, env, layout, []*Variable{lane, warp}, 4)

	called := false
	hook := func(ctx *Context) (bool, error) {
		called = true
		if ctx.Lane == nil || ctx.Lane.Name() != "lane" {
			t.Errorf("hook context lane = %v, want lane", ctx.Lane)
		}
		if ctx.Window != 32 {
			t.Errorf("hook context window = %d, want 32", ctx.Window)
		}
		return false, nil // decline: coverage fallback runs
	}
	if err := Smem(env, SmemCoalesce{VariableFamily: "lane", TrySwizzle: hook},
		[]*Variable{lane, warp}, 4, layout); err != nil {
		t.Fatalf("Smem failed: %v", err)
	}
	if !called {
		t.Fatal("swizzle hook was not invoked")
	}
	if len(before) == len(elementOffsets(t, env, layout, []*Variable{lane, warp}, 4)) {
		// fallback ran and repaired
		ok, _ := SmemBankPredicate(env, layout, []*Variable{lane, warp}, lane, 4)
		if !ok {
			t.Fatal("coverage fallback did not resolve conflicts")
		}
	}
}

// A multi-variable family has no single warp sweep to measure — the pass
// must reject it loudly (a silent suffix fallback would claim success while
// the swept variable still strides into bank collisions), leaving the
// layout untouched.
func TestSmemMultiVariableFamilyRejected(t *testing.T) {
	env := *NewEnv()
	f := env.Factory
	laneA := f.Variable("lane", "lane0", 3)
	laneB := f.Variable("lane", "lane1", 8)
	warp := f.Variable("warp", "warp", 2)
	tile := NewConstHTuple([]Value{3, 2, 8})

	coord, err := Access(env, []*Variable{laneA, laneB, warp}, tile)
	if err != nil {
		t.Fatalf("Access failed: %v", err)
	}
	layout := &Layout{Coordinate: coord, Shape: tile}
	before := elementOffsets(t, env, layout, []*Variable{laneA, laneB, warp}, 4)

	err = Smem(env, SmemCoalesce{VariableFamily: "lane"},
		[]*Variable{laneA, laneB, warp}, 4, layout)
	if err == nil {
		t.Fatal("multi-variable family: expected error")
	}
	if !strings.Contains(err.Error(), "unique family variable") {
		t.Fatalf("unexpected error: %v", err)
	}
	after := elementOffsets(t, env, layout, []*Variable{laneA, laneB, warp}, 4)
	if len(before) != len(after) {
		t.Fatal("rejected Smem mutated the layout")
	}
}

// --------------------------------------------------------------------------------------
// SMEM property: over the full factorization suite, success implies bank
// conflict-freedom; failure implies bijection intact
// --------------------------------------------------------------------------------------

func TestSmemSoundnessExhaustive(t *testing.T) {
	const maxN = 48

	cases, okCount := 0, 0
	for n := 2; n <= maxN; n++ {
		for _, bov := range unorderedFactorizations(n, 2) {
			for _, tileShape := range orderedFactorizations(n) {
				shapes := []ConstHTuple{NewConstHTuple(intsToValues(tileShape))}
				if len(tileShape) >= 2 {
					inner := NewConstHTuple([]Value{Value(tileShape[0]), Value(tileShape[1])})
					vec := []*ConstHTupleValue{NewConstHTupleValueHTuple(inner)}
					for _, d := range tileShape[2:] {
						vec = append(vec, NewConstHTupleValueLeaf(Value(d)))
					}
					shapes = append(shapes, ConstHTuple{vec: vec})
				}

				for _, tile := range shapes {
					env := *NewEnv()
					f := env.Factory
					vars := make([]*Variable, 0, len(bov))
					var lane *Variable
					laneCount := 0
					for i, bound := range bov {
						family := "warp"
						if i > 0 || len(bov) == 1 {
							family = "lane"
						}
						v := f.Variable(family, varName(i), bound)
						vars = append(vars, v)
						if family == "lane" {
							laneCount++
							if lane == nil {
								lane = v
							}
						}
					}
					// the sweep predicate needs exactly one lane variable
					if laneCount != 1 || lane.UpperBound() < 2 {
						continue
					}

					coord, err := Access(env, vars, tile)
					if err != nil {
						continue
					}
					layout := &Layout{Coordinate: coord, Shape: tile}
					cases++

					err = Smem(env, SmemCoalesce{VariableFamily: "lane"},
						vars, 4, layout)
					if err != nil {
						// conservative bailout: bijection intact
						assertBijective(t, env, layout, vars, 4)
						continue
					}
					okCount++
					assertBijective(t, env, layout, vars, 4)
					conflictFree, err := SmemBankPredicate(env, layout, vars, lane, 4)
					if err != nil {
						t.Fatalf("N=%d bov=%v tile=%v: predicate failed: %v",
							n, bov, tileShape, err)
					}
					if !conflictFree {
						t.Fatalf("N=%d bov=%v tile=%v: Smem success but banks conflicted",
							n, bov, tileShape)
					}
				}
			}
		}
	}
	t.Logf("Smem property: %d cases, %d conflict-free successes, %d bailouts",
		cases, okCount, cases-okCount)
}
