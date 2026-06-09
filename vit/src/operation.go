package vit

import (
	"context"

	"vit/internal/opcontext"
	"vit/internal/types"
)

// Operation describe the process every vit operation (commit / tag etc...) go through
//
// operation needs to be split like this because of the specifity of versionning vfx data:
// it could be copying hundreds of gigs, and therefore taking a really long time.
//
// The steps can be sumed up like this:
//   - 1. ensure operation is valid by checking the database.
//     (any concurrent ongoing operation conflict is handled at Transaction level)
//   - 2. if good: do the heavy lifting: update the blob (usally copy data to it).
//   - 3. if blob sucessfully updated, update the DB accordingly
//   - 4. Modify the workspace to present the changes done by the operation
//
// Naively this could be done in a single function. But this could imply:
//   - 1. getting RW acess on some part of the DB.
//   - 2. doing the blob update phase (pottentially 10's of minutes)
//   - 3. modifying the DB and releasing the RW privilege.
//
// Keeping RW acess on the DB for the duration of the blob update is a no go in regard of
// the size of the vfx data.
//
// So the main puprose of the Operation is to unforce this flow:
//   - 1. Validate:
//     -- executed at 'validation' of Transaction.
//     This means the Transaction is not yet created and will be rejected if an error is returned.
//     We prefer reject a Transaction rather than building a faulty one that will fail.
//     -- Read only acess on DB: we can't afford to keep RW access for the duration of the blob update!
//   - 2. BlobUpdate: do the heavy copying
//   - 3. DbUpdate: Gain acess on the DB with RW privilege and update it.
//   - 4. Finish: Update workspace, rebuild cache etc...
//
// This way, Exclusive writing on DB only happen on phase 3, maximising the number of concurrent
// operation that can happen on the same asset.
//
// The problem that could emerge is that :
//   - on phase 3, DB is updated based on data gathered on 1.
//   - But between 1. and 3. several minutes could have happened and DB may have changed significitvaly.
//     Even in a way that data from 1. can be in conclict.
//
// This Problem is adressed by the Transaction layer:
//   - Transaction ensures that 2 conflicting operation can't happen concurrently.
//   - So the DB can change a lot between 1. and 3. but Transaction ensure that it can't change
//     in a way that would make data from 1. irrelevant in regard of the ongoing operation.
//
// Splitting an operaiton like this also allow to combine multiple operations in a single command
// (committing when taggin a branch for instnace)
type Operation interface {
	// Validate shall gather data from db (using read only access)
	// Its puprose is:
	//		- 1. ensure the operation is valid (mainly validate user input)
	//          This function is to be executed in the "validate" function of transaction object.
	//          Rejecting invalid operation at that moment prevents creating invalid transaction.
	//      - 2. Gather all the necessary data to execute "BlobUpdate" fonction.
	//           During BlobUpdate, no access to Db so important information has to be gather here
	//			 and stored into the Operation struct.
	Validate(ctx context.Context, opctx *opcontext.OperationContext) error

	// BlobUpdate update the large binaries files inside vit repository.
	// No DB acess here, use data only saved in Operation at Validate phase.
	BlobUpdate(ctx context.Context, opctx *opcontext.OperationContext, c *Client) error

	// DBUpdate update a given asset
	// - receives an asset with RW privilege.
	// - Shan't write the file! just update the asset, writing on the Db is done at transaction level
	DBUpdate(opctx *opcontext.OperationContext, asset *types.Asset) error

	// Finish is executed after the database was successfully executed. from here: best effort only.
	// - Presenting new files to worksapce
	// - Rebuilding some cache.
	Finish(ctx context.Context, opctx *opcontext.OperationContext, c *Client) error
}

// An example of a how two operation can be executed in the same command.
// (for instance: when tagging a branch, we actually do commit the branch and tag the new commit)
// func exampleFunc( ctx context.Context, path string) error {
//
// 	err := opcontext.WithOperationContext() error {
//
// 		op1 := Operation()
// 		op2 := Operation()
//
// 		return WithTransactionEditBranch() {
// 			func() error {
// 				op1.Validate()
// 				op2.Validate()
// 			},
// 			func() error {
//
// 				op1.BlobUpdate()
// 				op2.BlobUpdate()
//
// 				// Only update one json per operation (to ensure atomicity of the operation)
// 				// only move has to update two.
// 				assetJSON := GetAssetWithWritePrivilege()
// 				assetData := assetJSON.Data
//
// 				op1.DbUpdate(assetData)
// 				op2.DbUpdate(assetData)
//
// 				WriteAsset(assetJSON)
//
// 				op1.Finish()
// 				op2.Finish()
//
// 				UpdateGlobalHistory("something done")
// 			}
// 		}
// 	}
// }
