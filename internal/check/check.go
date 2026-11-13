// Package check provides the repo's invariant-assertion guards: Check
// (always-on) and DCheck (debug builds only). Invariant violations panic
// loudly instead of corrupting compiler state.
package check

import (
	"fmt"
	"reflect"
	"runtime"
)

// Set by build tags (see below).
var dcheckEnabled = false

func fail(kind string, a ...interface{}) {
	expr := "invariant violated"
	if len(a) > 0 {
		// comma-ok: a non-string first arg (or missing format args) must not
		// panic inside fail itself with a secondary error (WR-14)
		if format, ok := a[0].(string); ok {
			expr = fmt.Sprintf(format, a[1:]...)
		} else {
			expr = fmt.Sprint(a...)
		}
	}
	// +1 for fail, +1 for the caller of Check/Dcheck.
	// runtime.Caller(2) points at the original call site.
	_, file, line, ok := runtime.Caller(2)
	if !ok {
		panic(fmt.Sprintf("%s failed: %s", kind, expr))
	}
	panic(fmt.Sprintf("%s failed: %s (%s:%d)", kind, expr, file, line))
}

// Check always panics when cond is false (release + debug).
func Check(cond bool, a ...interface{}) {
	if !cond {
		fail("CHECK", a...)
	}
}

func CheckNot(cond bool, a ...interface{}) {
	if cond {
		fail("CHECK_NOT", a...)
	}
}

// Unreachable always panics (marks a code path that must never execute).
func Unreachable(a ...interface{}) {
	fail("UNREACHABLE", a...)
}

// Bug always panics (marks an internal logic error).
func Bug(a ...interface{}) {
	fail("BUG", a...)
}

// DCheck panics only when dcheckEnabled is true (debug builds).
func DCheck(cond bool, a ...interface{}) {
	if !dcheckEnabled {
		return
	}
	if !cond {
		fail("DCHECK", a...)
	}
}

func CheckIf(precondition, cond bool, a ...interface{}) {
	if precondition {
		Check(cond, a...)
	}
}

// isNilValue reports whether v is nil, including typed-nil pointers,
// interfaces, maps, slices, funcs and channels (a bare == nil misses
// typed nils).
func isNilValue(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Ptr, reflect.Interface, reflect.Map, reflect.Slice,
		reflect.Func, reflect.Chan:
		return rv.IsNil()
	default:
		return false
	}
}

// CheckNil always panics when v is nil (or typed-nil).
func CheckNil(v any, a ...interface{}) {
	if !isNilValue(v) {
		fail("CHECK_NIL", a...)
	}
}

// CheckNotNil always panics when v is NOT nil (or typed-nil).
func CheckNotNil(v any, a ...interface{}) {
	if isNilValue(v) {
		fail("CHECK_NOT_NIL", a...)
	}
}

func inBounds[T any](index int, slice []T) bool {
	return index >= 0 && index < len(slice)
}

func CheckInBounds[T any](idx int, slice []T, a ...interface{}) {
	if !inBounds(idx, slice) {
		fail("out of bounds", a...)
	}
}
