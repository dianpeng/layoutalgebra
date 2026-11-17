package layoutalgebra

import "testing"

func validProblem() *Problem {
	return &Problem{
		Tile: Tile{Shape: []int{7, 16, 4}, DTypeSize: 4},
		Concurrency: []Variable{
			{Family: "warp", Name: "warp", Bound: 14},
			{Family: "lane", Name: "lane", Bound: 32},
		},
		Lane: "lane",
	}
}

func TestProblemValidate(t *testing.T) {
	if err := validProblem().Validate(); err != nil {
		t.Fatalf("valid problem rejected: %v", err)
	}

	bad := []struct {
		name   string
		mutate func(*Problem)
	}{
		{"bad dtype", func(p *Problem) { p.Tile.DTypeSize = 3 }},
		{"empty shape", func(p *Problem) { p.Tile.Shape = nil }},
		{"zero extent", func(p *Problem) { p.Tile.Shape[1] = 0 }},
		{"no concurrency", func(p *Problem) { p.Concurrency = nil }},
		{"bound 0", func(p *Problem) { p.Concurrency[0].Bound = 0 }},
		{"duplicate name", func(p *Problem) { p.Concurrency[1].Name = "warp" }},
		{"lane not in bov", func(p *Problem) { p.Lane = "tid" }},
	}
	for _, c := range bad {
		p := validProblem()
		c.mutate(p)
		if err := p.Validate(); err == nil {
			t.Errorf("%s: expected error", c.name)
		}
	}
}

func TestProblemVariable(t *testing.T) {
	p := validProblem()
	if v := p.Variable("lane"); v == nil || v.Bound != 32 {
		t.Errorf("Variable(lane) = %v, want bound 32", v)
	}
	if v := p.Variable("nope"); v != nil {
		t.Errorf("Variable(nope) = %v, want nil", v)
	}
}

type fakeAlgebra struct{ name string }

func (self fakeAlgebra) Name() string            { return self.name }
func (self fakeAlgebra) Supported(*Problem) bool { return true }
func (self fakeAlgebra) Access(*Problem) (Mapping, error) {
	return Mapping{Algebra: self.name}, nil
}
func (self fakeAlgebra) StrideMap(Mapping, *Problem) (Offset, error) {
	return Offset{Algebra: self.name}, nil
}
func (self fakeAlgebra) GmemOptimize(m Mapping, _ *Problem) (Mapping, error) {
	return m, nil
}
func (self fakeAlgebra) SmemOptimize(m Mapping, _ *Problem) (Mapping, error) {
	return m, nil
}

func TestRegistry(t *testing.T) {
	// fresh state: the registry is package-global, so use names unique to this
	// test and restore afterwards
	const name = "test-fake"
	defer delete(registry, name)

	if _, ok := Lookup(name); ok {
		t.Fatalf("%q already registered", name)
	}
	Register(fakeAlgebra{name: name})

	got, ok := Lookup(name)
	if !ok || got.Name() != name {
		t.Errorf("Lookup(%q) = %v, %v", name, got, ok)
	}

	names := Names()
	found := false
	for _, n := range names {
		if n == name {
			found = true
		}
	}
	if !found {
		t.Errorf("Names() = %v, missing %q", names, name)
	}

	defer func() {
		if recover() == nil {
			t.Error("duplicate Register: expected panic")
		}
	}()
	Register(fakeAlgebra{name: name})
}

func TestRegisterNilPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("Register(nil): expected panic")
		}
	}()
	Register(nil)
}
