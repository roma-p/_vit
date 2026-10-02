package types

type VitCommand string

const (
	CommandInit       VitCommand = "init"
	CommandAdd        VitCommand = "add"
	CommandCommit     VitCommand = "commit"
	CommandTag        VitCommand = "tag"
	CommandSourceAdd  VitCommand = "source-add"
	CommandSourcePull VitCommand = "source-pull"
	CommandSourcePush VitCommand = "source-push"
)
