package types

type ProgressWriter interface {
	InitProgress(manifest ProgressManifest)
	UpdateProgress(updatedItem ProgressItem)
	CloseProgress(progressFinish ProgressFinish)
}

type ProgressWriterEmpty struct{}

func (p *ProgressWriterEmpty) InitProgress(manifest ProgressManifest)      {}
func (p *ProgressWriterEmpty) UpdateProgress(updatedItem ProgressItem)     {}
func (p *ProgressWriterEmpty) CloseProgress(progressFinish ProgressFinish) {}

type ProgressItem struct {
	Name string `json:"name"`
	Size int    `json:"size"`
}

type ProgressManifest struct {
	Operation string `json:"operation"`
	Items     string `json:"items"`
}

type ProgressFinish struct {
	Status bool `json:"status"`
}
