package types

type SourceUpdate struct {
	Name      string
	RefSpec   *Ref
	OldTarget *Ref
	OldCommit string
	NewTarget *Ref
	NewCommit string
	SrcUID    string
}
