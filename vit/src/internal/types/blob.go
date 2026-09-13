package types

// BlobUpdatePlan describes the plan to be executed by blob.UpdateBlob
//   - list of blob copy to execute.
//   - list of content nodes to write to register the blob copy.
//     (only the root nodes are passed, all children reccursively written)
//   - list of the optionnal manifest sidecar json to describes the blob obects copied.
type BlobUpdatePlan struct {
	CopyRequests []CopyFilesRequest
	ContentRoots []ContentRootWrite
	Manifests    []ManifestWrite
}

func (p *BlobUpdatePlan) IsEmpty() bool {
	return p == nil ||
		(len(p.CopyRequests) == 0 && len(p.ContentRoots) == 0 && len(p.Manifests) == 0)
}

type ContentRootWrite struct {
	ContentRootPath string
	Root            *ContentNode
}

type ManifestWrite struct {
	CommitBlobPath string
	Manifest       Manifest
}
