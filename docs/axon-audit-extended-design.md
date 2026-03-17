# Axon Audit Extended Design

## 1. Goals and Background

The current `axon audit` command provides basic security scanning (supporting vulnerability discovery and interactive fixing via `--fix`). However, when reviewing Mules/Skills from external or untrusted sources, it lacks sufficient depth of review, pre-warning for unauthorized permission actions, and intuitive risk prompts for users.

This design is inspired by the strict `Skill Vetter` specification, aiming to further enhance `axon audit`, upgrading it to an **in-depth static permission probe** and an **automated security verdict system**. During this process, we maintain command and terminology consistency (using `Audit` instead of introducing the `Vetting` concept), while discarding unreliable network source tracking, focusing entirely on essential static code analysis.

## 2. Core Improvements

### 2.1 Enhance LLM Prompts: Precise Targeting of High-Risk "Red Flags"

The current system detection points (Secrets, Injection, Exfiltration, PII) lean towards generic vulnerabilities. The prompts should be expanded to specifically highlight malicious attack vectors common in Agent/Skill environments:
* **Unauthorized Resource Access**: Explicitly flag any behavior attempting to read or write sensitive configuration file system paths (e.g., `~/.ssh`, `~/.aws`, `~/.config`, `/etc/passwd`, etc.).
* **Core Memory Threats**: Intercept actions attempting to tamper with or steal core Agent architecture files (e.g., `MEMORY.md`, `IDENTITY.md`, `SOUL.md`, etc.).
* **Obfuscation and Evasion**: Identify external code calls decoded using `base64` or highly obfuscated code as high risk.
* **Dangerous System Operations**: Flag any behavior containing or requesting privilege escalation (`sudo`), directly calling dangerous OS-level commands (e.g., `rm -rf`), or attempting to mount or operate system processes.

### 2.2 Data Model Upgrade: Introduction of "Permission Scope"

Traditional audit reports only contain "where and what type of problem exists (Findings)" in the code. The extended design requires the LLM to extract the "permission scope" the code is attempting to request, in addition to returning Findings. This is similar to permission declarations when installing mobile apps:
* **File Reads**: What other files outside the runtime environment is the code trying to read?
* **File Writes**: Which paths is the code trying to modify, delete, or write to?
* **Network**: What external addresses are requested? (Hardcoded IPs in particular are an abnormal red flag).
* **Commands**: Which OS-level commands are explicitly called (`curl`, `wget`, `bash`, etc.)?

Structural Refactoring Reference:
```go
type PermissionScope struct {
	FileReads  []string `json:"file_reads"`
	FileWrites []string `json:"file_writes"`
	Network    []string `json:"network"`
	Commands   []string `json:"commands"`
}

type FileAuditResult struct {
	Findings    []Finding       `json:"findings"`
	Permissions PermissionScope `json:"permissions"`
}
```

### 2.3 Risk Reclassification and Automated Final Verdict

The current three-level classification (High, Medium, Low) holds insufficient execution power for novice users or non-security experts. Therefore, the following mechanisms should be added to the system:
1. **EXTREME Level**: For example, attempts to access `~/.ssh` should bypass general high risk and be directly marked as extreme risk (zero-tolerance behavior).
2. **VERDICT**: Automatically calculate final recommendations based on the number and level of vulnerabilities found (a summary conclusion):
    * `SAFE TO INSTALL / RUN`: Clean code with only a few or no low/medium risk items.
    * `INSTALL WITH CAUTION`: High-level issues that must be confirmed by humans exist; manual review is recommended.
    * `⛔ DO NOT INSTALL / RUN`: Extreme-level issues or clear malicious features exist; the system intercepts or strongly warns against usage.

### 2.4 Audit Report UI View Revamp

The final console output interface needs completely abandon the pure list-style warning presentation and change to a structured report model:

```text
SECURITY AUDIT REPORT
═══════════════════════════════════════
Target: [target name]
Files Reviewed: [count]
───────────────────────────────────────
RED FLAGS FOUND: [count]
• [EXTREME] (L42): Attempted to access system credential files (~/.ssh/id_rsa)
• [HIGH] (L105): Found external code execution call obfuscated via Base64

PERMISSIONS REQUIRED (Estimated):
• File Reads: [paths or "None"]
• Network: [IPs/Domains or "None"]
• Commands: [Commands or "None"]
───────────────────────────────────────
RISK LEVEL: [LOW / MEDIUM / HIGH / EXTREME]

VERDICT: [SAFE TO INSTALL / CAUTION / DO NOT INSTALL]
═══════════════════════════════════════
```

## 3. Implementation Roadmap

The transformation can be fully self-contained within the current codebase (without introducing external dependencies):
1. Revise the Prompt template passed to the LLM in `src/internal/audit/auditor.go` to include red flag detection rules and the JSON return structure (adding the Permission node).
2. Update `src/internal/audit/types.go` and logic of the JSON parser to accommodate the new two-layer structure and handle merged displays.
3. Improve CLI view printing methods like `printFindings` in `src/cmd/audit.go` to add aggregated views and the `VERDICT` smart calculation recommendation logic.
4. Clean up test cases relying on the old structure and cache structures, ensuring backward compatibility or automatically ignoring invalid upgrades.
