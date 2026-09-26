package noop_test

import (
	"context"
	"testing"

	"github.com/Gsc23/donkey/adapter/noop"
)

type fakeSeeder struct{ id string }

func (f fakeSeeder) ID() string             { return f.id }
func (f fakeSeeder) Dependencies() []string { return nil }

func TestAdapter_LogsIDWithoutExecuting(t *testing.T) {
	var logged []string
	a := noop.New()
	a.Log = func(id string) { logged = append(logged, id) }

	if err := a.Execute(context.Background(), fakeSeeder{id: "x"}); err != nil {
		t.Fatal(err)
	}
	if len(logged) != 1 || logged[0] != "x" {
		t.Fatalf("esperava [x], veio %v", logged)
	}
}
