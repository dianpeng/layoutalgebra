package check

import (
  "fmt"
  "strings"
  "testing"
)

// WR-14: check.fail asserted a[0].(string) unchecked — a non-string first
// arg panicked inside fail with a confusing secondary error.

func TestCheckNonStringMessage(t *testing.T) {
  defer func() {
    r := recover()
    if r == nil {
      t.Fatal("expected panic from Check(false, 42)")
    }
    msg, ok := r.(string)
    if !ok {
      t.Fatalf("panic value is %T (%v), want string message", r, r)
    }
    if !strings.Contains(msg, "CHECK failed") {
      t.Errorf("panic message %q does not contain %q", msg, "CHECK failed")
    }
    if strings.Contains(msg, "%!") {
      t.Errorf("panic message %q contains %%! formatting garbage", msg)
    }
  }()
  Check(false, 42)
}

func TestCheckNormalMessageStillWorks(t *testing.T) {
  defer func() {
    r := recover()
    if r == nil {
      t.Fatal("expected panic from Check with a format string and args")
    }
    msg, ok := r.(string)
    if !ok {
      t.Fatalf("panic value is %T (%v), want string message", r, r)
    }
    if !strings.Contains(msg, "boom 7") {
      t.Errorf("panic message %q does not contain formatted %q", msg, "boom 7")
    }
  }()
  Check(false, fmt.Sprintf("boom %d", 7))
}
