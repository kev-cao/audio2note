package registry

import (
	"reflect"
	"strings"
	"testing"
)

type item struct{ name string }

func (i item) Name() string { return i.name }

func TestRegisterAndGet(t *testing.T) {
	r := New[item]()
	r.Register(item{name: "a"})
	r.Register(item{name: "b"})

	got, err := r.Get("a")
	if err != nil {
		t.Fatalf("Get(a) err = %v", err)
	}
	if got.name != "a" {
		t.Errorf("Get(a) = %q, want %q", got.name, "a")
	}
}

func TestGetUnknownListsAvailable(t *testing.T) {
	r := New[item]()
	r.Register(item{name: "alpha"})
	r.Register(item{name: "beta"})

	_, err := r.Get("missing")
	if err == nil {
		t.Fatal("expected error for unknown name")
	}
	msg := err.Error()
	for _, want := range []string{"missing", "alpha", "beta"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message %q missing %q", msg, want)
		}
	}
}

func TestNamesSorted(t *testing.T) {
	r := New[item]()
	r.Register(item{name: "charlie"})
	r.Register(item{name: "alpha"})
	r.Register(item{name: "bravo"})

	got := r.Names()
	want := []string{"alpha", "bravo", "charlie"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Names() = %v, want %v", got, want)
	}
}

func TestRegisterDuplicatePanics(t *testing.T) {
	r := New[item]()
	r.Register(item{name: "a"})

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on duplicate Register")
		}
	}()
	r.Register(item{name: "a"})
}

func TestEmptyRegistry(t *testing.T) {
	r := New[item]()
	if got := r.Names(); len(got) != 0 {
		t.Errorf("Names() on empty registry = %v, want []", got)
	}
	if _, err := r.Get("anything"); err == nil {
		t.Error("expected error on Get from empty registry")
	}
}
