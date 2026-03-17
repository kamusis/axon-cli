package audit

// Finding represents a security issue found during audit.
type Finding struct {
	FilePath    string `json:"file_path"`
	LineNumber  int    `json:"line_number"`
	IssueType   string `json:"issue_type"`   // "secret", "injection", "exfiltration", "pii", "privilege"
	Severity    string `json:"severity"`     // "extreme", "high", "medium", "low"
	Description string `json:"description"`
	Snippet     string `json:"snippet"`
}

// PermissionScope captures the capabilities claimed or exercised by the analyzed code.
// Each field is a deduplicated list of resources/addresses/commands observed.
type PermissionScope struct {
	FileReads  []string `json:"file_reads"`
	FileWrites []string `json:"file_writes"`
	Network    []string `json:"network"`
	Commands   []string `json:"commands"`
}

// FileAuditResult holds the security findings and estimated permission scope for a single file.
type FileAuditResult struct {
	Findings    []Finding       `json:"findings"`
	Permissions PermissionScope `json:"permissions"`
}
