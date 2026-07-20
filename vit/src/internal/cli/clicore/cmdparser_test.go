package clicore

import (
	"testing"

	testutils "vit/internal/testhelpers"
)

func TestOnlyPosArgs(t *testing.T) {
	parser := NewArgParser("add", []string{"assetPath", "src"}, []string{})
	testutils.AssertEqual(t, parser.Parse([]string{"some/path", "source.ma"}), nil)
	testutils.AssertEqual(t, parser.Arg("assetPath"), "some/path")
	testutils.AssertEqual(t, parser.Arg("src"), "source.ma")

	testutils.AssertError(t, parser.Parse([]string{"some/path"}))
}

func TestOnlyPosArgsAndOptionnalArgs(t *testing.T) {
	parser := NewArgParser("add", []string{"assetPath"}, []string{"src"})

	testutils.AssertEqual(t, parser.Parse([]string{"some/path", "source.ma"}), nil)
	testutils.AssertEqual(t, parser.Arg("assetPath"), "some/path")
	testutils.AssertEqual(t, parser.Arg("src"), "source.ma")

	testutils.AssertEqual(t, parser.Parse([]string{"some/path"}), nil)
	testutils.AssertEqual(t, parser.Arg("assetPath"), "some/path")
	testutils.AssertEqual(t, parser.Arg("src"), "")
}

func TestOnlyPosArgsAndOptionnalArgsAndFlags(t *testing.T) {
	parser := NewArgParser("add", []string{"assetPath"}, []string{"src"})
	parser.Bool("verbose", false, "some usage")

	testutils.AssertEqual(t, parser.Parse([]string{"some/path", "source.ma", "-verbose"}), nil)
	testutils.AssertEqual(t, parser.Arg("assetPath"), "some/path")
	testutils.AssertEqual(t, parser.Arg("src"), "source.ma")
	testutils.AssertEqual(t, *parser.Flag("verbose").(*bool), true)

	testutils.AssertEqual(t, parser.Parse([]string{"some/otherpath"}), nil)
	testutils.AssertEqual(t, parser.Arg("assetPath"), "some/otherpath")
	testutils.AssertEqual(t, parser.Arg("src"), "")
	testutils.AssertEqual(t, *parser.Flag("verbose").(*bool), false)
}
