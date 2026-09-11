package transaction

import (
	"context"
	"log/slog"

	"vit/internal/db"
	"vit/internal/opcontext"
	"vit/internal/types"
	"vit/internal/workspace"
)

// TODO: logger interface to change: sometimes i pass a logger, othertime an interface....

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
//   - 2. PlanBlobUpdate: pure compute, no I/O — describe the object-tier writes as a
//     BlobUpdatePlan. The transaction executes it (the heavy copying) via blob.UpdateBlob.
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
	//      - 2. Gather all the necessary data to execute "PlanBlobUpdate" fonction.
	//           During PlanBlobUpdate, no access to Db so important information has to be gather here
	//			 and stored into the Operation struct.
	Validate(ctx context.Context, opctx *opcontext.OperationContext, dbClient *db.Client) error

	// PlanBlobUpdate computes the object-tier writes (blob copies, content nodes,
	// manifests) and returns them as a BlobUpdatePlan. Pure compute: no I/O and
	// no DB access — use only data saved into the Operation during Validate. The
	// transaction executes the plan (the heavy copying) via blob.UpdateBlob.
	PlanBlobUpdate(
		ctx context.Context,
		opctx *opcontext.OperationContext,
	) (*types.BlobUpdatePlan, error)

	// DBUpdate update a given asset
	// - receives an asset with RW privilege.
	// - Shan't write the file! just update the asset, writing on the Db is done at transaction level
	DBUpdate(
		ctx context.Context,
		opctx *opcontext.OperationContext,
		dbClient *db.Client,
	) error

	// Finish is executed after the database was successfully executed. from here: best effort only.
	// - Presenting new files to worksapce
	// - Rebuilding some cache.
	Finish(
		ctx context.Context,
		opctx *opcontext.OperationContext,
		dbClient *db.Client,
		logger *slog.Logger,
		p workspace.Presenter,
	)
}
