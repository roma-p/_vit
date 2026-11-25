package types

type QueryRequest struct {
	With     []string          `json:"with,omitempty"`
	Without  []string          `json:"without,omitempty"`
	Eq       map[string]string `json:"eq,omitempty"`
	Fetch    []string          `json:"fetch,omitempty"`
	Asset    bool              `json:"asset,omitempty"`
	Distinct bool              `json:"distinct,omitempty"`
	Set      bool              `json:"set,omitempty"`
}

type QueryResult struct {
	Rows []QueryRow          `json:"rows,omitempty"`
	Sets map[string][]string `json:"sets,omitempty"`
}

type QueryRow struct {
	Asset string            `json:"asset,omitempty"`
	Attrs map[string]string `json:"attrs,omitempty"`
}
