---
name: policy-comparator
description: >-
  Compare policy documents side by side. Produces a structured
  comparison matrix. Use when analyzing policies from multiple
  sources against a baseline.
license: Apache-2.0
metadata:
  author: Red Hat ET
  category: human-resources
---

# Policy comparator skill

When asked to compare policy documents:

1. Use `read_file` to read the baseline policy (your organization's
   current policy or the standard you are comparing against)
2. Use `read_file` to read each comparison document
3. Identify the key policy dimensions by scanning all documents:
   - Coverage areas (e.g., eligibility, duration, benefits, exceptions)
   - Quantitative terms (e.g., days, percentages, dollar amounts)
   - Qualitative terms (e.g., approval process, escalation path)
4. For each dimension, classify each document's position relative
   to the baseline:
   - **Above**: more generous or comprehensive than baseline
   - **At**: equivalent to baseline
   - **Below**: less generous or comprehensive than baseline
   - **N/A**: dimension not addressed in this document
5. Produce the output as a comparison matrix:

   | Dimension | Baseline | Doc A | Doc B | Doc C |
   |-----------|----------|-------|-------|-------|
   | Eligibility | All FTE | Above | At | Below |

6. After the matrix, provide:
   - A summary paragraph highlighting where the baseline is
     strongest and weakest relative to the comparison set
   - Specific recommendations for policy updates, if requested
7. Use `write_file` to save the comparison if requested

## Important guidelines

- Be precise about quantitative differences — "12 weeks vs 16 weeks"
  not just "below"
- When a document is ambiguous about a dimension, quote the relevant
  text and mark it as "Unclear" rather than guessing
- Do not make value judgments about which policy is "better" unless
  explicitly asked — present the facts and let the analyst decide
- If documents are in different formats (PDF text, DOCX, markdown),
  normalize the comparison — format should not affect the analysis
