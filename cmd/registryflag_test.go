package cmd

import (
	"strings"
	"testing"
)

func TestRegistryValue_Set_Valid(t *testing.T) {
	var target string
	v := &registryValue{target: &target, allowed: func() []string { return []string{"a", "b", "c"} }}
	if err := v.Set("b"); err != nil {
		t.Fatalf("Set(\"b\") err = %v", err)
	}
	if target != "b" {
		t.Errorf("target = %q, want %q", target, "b")
	}
}

func TestRegistryValue_Set_Invalid(t *testing.T) {
	target := "a"
	v := &registryValue{target: &target, allowed: func() []string { return []string{"a", "b", "c"} }}
	err := v.Set("bogus")
	if err == nil {
		t.Fatal("expected error for invalid value")
	}
	msg := err.Error()
	for _, want := range []string{"must be one of", "bogus", "a", "b", "c"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing substring %q", msg, want)
		}
	}
	// Target must not be mutated on failure.
	if target != "a" {
		t.Errorf("target was mutated on invalid Set: %q", target)
	}
}

func TestRegistryValue_String(t *testing.T) {
	target := "current"
	v := &registryValue{target: &target, allowed: func() []string { return nil }}
	if got := v.String(); got != "current" {
		t.Errorf("String() = %q, want %q", got, "current")
	}
}

func TestRegistryValue_Type(t *testing.T) {
	v := &registryValue{}
	if got := v.Type(); got != "string" {
		t.Errorf("Type() = %q, want \"string\"", got)
	}
}
