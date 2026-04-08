---
name: code-review
description: Review code files for bugs, style, and security issues
---

# Code review skill

When asked to review code:

1. Use `read_file` to read the file(s) to review
2. Analyze for:
   - **Bugs**: logic errors, off-by-one, nil derefs
   - **Security**: injection, hardcoded secrets, unsafe input handling
   - **Style**: naming, formatting, idiomatic patterns
3. Report findings with severity (Critical / Important / Minor)
4. Include file path and line references
5. Suggest fixes for Critical and Important issues
