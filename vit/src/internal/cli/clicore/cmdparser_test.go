package clicore

import (
	"testing"

	testutils "vit/internal/testhelpers"
)

func TestOnlyPosArgs(t *testing.T) {
	parser := NewArgParser("add", []string{"assetPath", "src"}, []string{})
	testutils.AssertEqual(t, parser.Parse([]string{"some/path", "source.ma"}), nil)
	testutils.AssertEqual(t, parser.GetArg("assetPath"), "some/path")
	testutils.AssertEqual(t, parser.GetArg("src"), "source.ma")

	testutils.AssertError(t, parser.Parse([]string{"some/path"}))
}

func TestOnlyPosArgsAndOptionnalArgs(t *testing.T) {
	parser := NewArgParser("add", []string{"assetPath"}, []string{"src"})

	testutils.AssertEqual(t, parser.Parse([]string{"some/path", "source.ma"}), nil)
	testutils.AssertEqual(t, parser.GetArg("assetPath"), "some/path")
	testutils.AssertEqual(t, parser.GetArg("src"), "source.ma")

	testutils.AssertEqual(t, parser.Parse([]string{"some/path"}), nil)
	testutils.AssertEqual(t, parser.GetArg("assetPath"), "some/path")
	testutils.AssertEqual(t, parser.GetArg("src"), "")
}

func TestOnlyPosArgsAndOptionnalArgsAndFlags(t *testing.T) {
	parser := NewArgParser("add", []string{"assetPath"}, []string{"src"})
	parser.AddFlag("verbose", false, "some usage")

	testutils.AssertEqual(t, parser.Parse([]string{"some/path", "source.ma", "-verbose"}), nil)
	testutils.AssertEqual(t, parser.GetArg("assetPath"), "some/path")
	testutils.AssertEqual(t, parser.GetArg("src"), "source.ma")
	testutils.AssertEqual(t, *parser.GetFlag("verbose").(*bool), true)

	testutils.AssertEqual(t, parser.Parse([]string{"some/otherpath"}), nil)
	testutils.AssertEqual(t, parser.GetArg("assetPath"), "some/otherpath")
	testutils.AssertEqual(t, parser.GetArg("src"), "")
	testutils.AssertEqual(t, *parser.GetFlag("verbose").(*bool), false)
}

func TestVariadicArgs(t *testing.T) {
	parser := NewArgParser("source pull", []string{"path"}, []string{})
	if err := parser.AddVariadic("names", "source names"); err != nil {
		t.Fatalf("AddVariadic: %v", err)
	}

	// zero trailing → required mapped, empty variadic (no panic)
	testutils.AssertEqual(t, parser.Parse([]string{"some/path"}), nil)
	testutils.AssertEqual(t, parser.GetArg("path"), "some/path")
	testutils.AssertEqual(t, len(parser.GetVariadic("names")), 0)

	// one trailing
	testutils.AssertEqual(t, parser.Parse([]string{"some/path", "a"}), nil)
	testutils.AssertEqual(t, len(parser.GetVariadic("names")), 1)

	// many trailing
	testutils.AssertEqual(t, parser.Parse([]string{"some/path", "a", "b", "c"}), nil)
	got := parser.GetVariadic("names")
	testutils.AssertEqual(t, len(got), 3)
	testutils.AssertEqual(t, got[0], "a")
	testutils.AssertEqual(t, got[2], "c")

	// missing the required positional → clean error, NOT a panic
	testutils.AssertError(t, parser.Parse([]string{}))
}

func TestVariadicRejectsOptional(t *testing.T) {
	parser := NewArgParser("x", []string{"path"}, []string{"opt"})
	if err := parser.AddVariadic("names", "..."); err == nil {
		t.Fatal("expected AddVariadic to reject: can't have variadic + optional positionals")
	}
}
