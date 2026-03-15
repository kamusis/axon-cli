package audit

// Finding represents a security issue found during audit.
type Finding struct {
	FilePath    string `json:"file_path"`
	LineNumber  int    `json:"line_number"`
	IssueType   string `json:"issue_type"`   // "secret", "injection", "exfiltration", "pii"
	Severity    string `json:"severity"`     // "high", "medium", "low"
	Description string `json:"description"`
	Snippet     string `json:"snippet"`
}
