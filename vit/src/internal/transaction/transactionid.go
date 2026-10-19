package transaction

import (
	"fmt"
	"net/url"
	"strings"

	"vit/internal/types"
)

type TransactionID struct {
	Operation TransactionOperation
	Ref       *types.Ref
}

type TransactionOperation string

const (
	TransactionTreeChanged TransactionOperation = "TreeChanged"
	TransactionCommit      TransactionOperation = "Commit"
	TransactionBranch      TransactionOperation = "Branch"
	TransactionTag         TransactionOperation = "Tag"
)

func (t *TransactionID) Encode() string {
	encoded := url.QueryEscape(t.Ref.AssetPath)
	encoded = strings.ReplaceAll(encoded, "+", "%20") + "---" + string(t.Operation)
	if t.Ref.RefType != types.RefTypeEmpty {
		encoded += "---" + t.Ref.ObjectPath()
	}
	return encoded
}

func newTransactionID(op TransactionOperation, ref *types.Ref) TransactionID {
	return TransactionID{
		Operation: op,
		Ref:       ref,
	}
}

func newTransactionIDFromName(repoPath, encodedName string) (*TransactionID, error) {
	splitTransactionID := strings.Split(encodedName, "---")

	if len(splitTransactionID) < 2 || len(splitTransactionID) > 3 {
		return nil, fmt.Errorf("not a valid transaction id: %s", encodedName)
	}

	decoded := strings.ReplaceAll(splitTransactionID[0], "%20", "+")
	decoded, err := url.QueryUnescape(decoded)
	if err != nil {
		return nil, fmt.Errorf("failed to decode transaction id '%s' : %w", encodedName, err)
	}

	operation := splitTransactionID[1]
	var ref *types.Ref
	if len(splitTransactionID) == 3 {
		refSubPath := splitTransactionID[2]
		refPath := decoded + refSubPath
		ref, err = types.NewRefFromPath(repoPath, refPath)
		if err != nil {
			return nil, err
		}
	} else {
		ref = types.NewRefEmpty(repoPath, decoded)
	}

	return &TransactionID{
		Operation: TransactionOperation(operation),
		Ref:       ref,
	}, nil
}
