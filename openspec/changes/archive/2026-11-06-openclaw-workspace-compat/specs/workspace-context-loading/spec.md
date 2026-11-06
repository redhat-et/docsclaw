## ADDED Requirements

### Requirement: Load OpenClaw bootstrap files from workspace

The system SHALL discover and read the following files from the
workspace directory at session start, in this order:

1. `AGENTS.md` -- operating instructions
2. `SOUL.md` -- persona and tone
3. `USER.md` -- user context
4. `IDENTITY.md` -- agent name and role
5. `TOOLS.md` -- tool guidance (advisory, not enforcement)

All files are optional. Missing files SHALL be skipped silently.

#### Scenario: All five files present

- **WHEN** the workspace contains AGENTS.md, SOUL.md, USER.md,
  IDENTITY.md, and TOOLS.md
- **THEN** the system SHALL load all five files and inject their
  content into the system prompt in the specified order

#### Scenario: Partial files present

- **WHEN** the workspace contains only SOUL.md and USER.md
- **THEN** the system SHALL load those two files and skip the
  remaining three without errors or warnings

#### Scenario: No OpenClaw files present

- **WHEN** the workspace contains no OpenClaw files
- **THEN** the system SHALL not inject any workspace context and
  the system prompt SHALL consist of only `system-prompt.txt` content

### Requirement: Workspace context appended as Project Context section

The system SHALL assemble loaded workspace files into a structured
"Project Context" section appended to the system prompt after the
base `system-prompt.txt` content. Each file SHALL be wrapped with
a markdown header identifying its source.

#### Scenario: Project Context section structure

- **WHEN** SOUL.md contains "Be direct and concise" and USER.md
  contains "Pavel, OCTO team"
- **THEN** the system prompt SHALL contain a section like:

  ```
  ## Project Context

  ### SOUL
  Be direct and concise

  ### USER
  Pavel, OCTO team
  ```

#### Scenario: system-prompt.txt and OpenClaw files coexist

- **WHEN** both `system-prompt.txt` and OpenClaw workspace files exist
- **THEN** `system-prompt.txt` content SHALL appear first, followed
  by the Project Context section (Option C: overlay model)

### Requirement: Per-file truncation limit

The system SHALL truncate any single workspace file that exceeds
20,000 characters and log a warning indicating the file was truncated.

#### Scenario: Large AGENTS.md truncated

- **WHEN** AGENTS.md contains 25,000 characters
- **THEN** the system SHALL use only the first 20,000 characters
  and log a warning with the file name and original size

### Requirement: Total context truncation limit

The system SHALL enforce a total limit of 60,000 characters across
all workspace files combined. If the total exceeds this limit, the
system SHALL truncate the combined content and log a warning.

#### Scenario: Combined files exceed total limit

- **WHEN** five workspace files total 75,000 characters (each under
  the per-file limit)
- **THEN** the system SHALL include files in order until the 60,000
  character budget is exhausted, truncating the last file that
  crosses the boundary

### Requirement: Log loaded workspace files

The system SHALL log which workspace files were loaded, including
the total character count, at INFO level.

#### Scenario: Logging loaded files

- **WHEN** SOUL.md (500 chars) and USER.md (300 chars) are loaded
- **THEN** the system SHALL log a message like:
  `"loaded workspace context" files=["SOUL.md","USER.md"] total_chars=800`
