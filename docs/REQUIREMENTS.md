# Requirements: Axon CLI

## 1. Problem Statement
The user operates multiple AI development tools (Windsurf, Antigravity, OpenCode, Neovate, etc.) across various platforms (Windows, macOS, Linux VPS). Each tool maintains its own local directory for "skills," "workflows," and "commands." This fragmentation leads to version inconsistency, manual synchronization overhead, and a high risk of losing custom-developed skills when switching environments.

## 2. User Stories

### Story 1: "Write Once, Run Everywhere"
As an AI Agent developer, I want to write a new skill in Windsurf on my Mac and have it instantly available in OpenClaw on my VPS without manual copying, so that I can maintain a high velocity of development.

### Story 2: "New Machine Rapid Onboarding"
As a power user, when I set up a new laptop, I want to run a single command that pulls all my existing agent skills and automatically connects them to my installed editors, so that I don't waste time hunting for directory paths.

### Story 3: "Hot-Reloading Development"
As a developer, I want to modify a skill file in my favorite IDE and see the changes reflected immediately in all running AI tools that use that skill, enabling a seamless "save-and-test" feedback loop.

### Story 4: "Cross-Platform Consistency"
As a multi-OS user, I want a single tool that abstracts away the path differences between Windows (`C:\...`) and Unix (`~/...`), providing a unified interface for skill management.

## 3. Functional Requirements
- **Centralized Storage**: All skills must reside in a single "Source of Truth" (a local Git repository).
- **Symlink Management**: The tool must be able to replace default tool directories with symbolic links pointing to the central repository.
- **Git Automation**: One-click (or one-command) synchronization with a remote private repository (Push/Pull/Commit).
- **Auto-Discovery**: Ability to detect common AI tool installation paths and suggest linking.
- **Status Reporting**: Show which tools are currently linked and if there are local uncommitted changes.

## 4. Non-Functional Requirements
- **Single Binary**: The tool must be a standalone executable with zero external runtime dependencies (no Python/Node required on target).
- **Performance**: Operations like linking and syncing must be near-instantaneous.
- **Robustness**: Atomic file operations; never delete user data without backup or confirmation.
- **Security**: Work exclusively with private Git repositories; handle sensitive environment variables safely.
- **Portability**: Support for Linux (x64/ARM), macOS (Intel/M1/M2), and Windows.
