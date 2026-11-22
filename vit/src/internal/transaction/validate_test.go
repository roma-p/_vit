package transaction

import (
	"testing"

	"vit/internal/types"
)

func TestCheckConflict(t *testing.T) {
	cases := []struct {
		a, b TransactionID
		want bool
	}{
		{tid(TransactionAttr, "seq/010"), tid(TransactionAttr, "seq/010"), true},
		{tid(TransactionAttr, "seq/010"), tid(TransactionAttr, "seq/020"), false},
		{tid(TransactionAttr, "seq/010"), branchTid("seq/010", "base"), false},
		{tid(TransactionAttr, "seq/010"), tid(TransactionTreeChanged, "seq/010"), true},
		{tid(TransactionAttr, "seq/010"), tid(TransactionTreeChanged, "seq"), true},
		{tid(TransactionAttr, "seq/010"), tid(TransactionTreeChanged, "seq/010/sh020"), false},
		{tid(TransactionAttr, "seq/010"), tid(TransactionTreeChanged, "seq/0100"), false},
		{tid(TransactionTreeChanged, "seq"), branchTid("seq/010", "base"), true},
		{tid(TransactionTreeChanged, "seq"), tid(TransactionTreeChanged, "seq/010"), true},
		{branchTid("x", "base"), branchTid("x", "wip"), false},
		{branchTid("x", "base"), branchTid("x", "base"), true},
	}
	for _, c := range cases {
		if got := checkConflict(c.a, c.b); got != c.want {
			t.Errorf("checkConflict(%s, %s) = %v, want %v", c.a.Encode(), c.b.Encode(), got, c.want)
		}
		if got := checkConflict(c.b, c.a); got != c.want {
			t.Errorf("checkConflict(%s, %s) = %v, want %v", c.b.Encode(), c.a.Encode(), got, c.want)
		}
	}
}

// -- FILE LOCAL -------------------------------------------------------------

const conflictRepo = "/repo"

func tid(op TransactionOperation, path string) TransactionID {
	return newTransactionID(op, types.NewRefEmpty(conflictRepo, path))
}

func branchTid(path, branch string) TransactionID {
	return newTransactionID(TransactionBranch, types.NewRefBranch(&types.RepoRef{Path: conflictRepo}, path, branch))
}
