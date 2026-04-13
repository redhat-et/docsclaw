---
name: resume-screener
description: >-
  Screen resumes against a job description. Scores candidates on
  qualification alignment, experience match, and fit. Use when HR
  uploads resumes for a job opening.
license: Apache-2.0
metadata:
  author: Red Hat ET
  category: human-resources
---

# Resume screener skill

When asked to screen or grade resumes against a job description:

1. Use `read_file` to read the job description document first
2. Extract the key requirements:
   - **Required qualifications** (must-have skills, certifications)
   - **Preferred experience** (years, domains, technologies)
   - **Cultural fit indicators** (leadership style, collaboration)
3. For each resume:
   - Use `read_file` to read the resume
   - Score on a 1-10 scale for each category:
     - **Qualification alignment**: how well required skills match
     - **Experience match**: relevance and depth of experience
     - **Fit**: alignment with role expectations and culture
   - Compute a weighted overall score (qualifications 40%,
     experience 35%, fit 25%)
   - Write a one-paragraph justification
4. Produce the final output as a ranked table:

   | Rank | Candidate | Overall | Quals | Exp | Fit | Justification |
   |------|-----------|---------|-------|-----|-----|---------------|

5. If there are more than 20 candidates, also provide a short list
   of the top 20 with a recommendation paragraph for each
6. Use `write_file` to save the results if requested

## Important guidelines

- Never disqualify candidates based on protected characteristics
- Flag gaps or concerns rather than making binary accept/reject
  decisions — the recruiter makes the final call
- If a resume is in a format you cannot parse, note it as
  "unreadable" with the filename and move to the next
- When qualifications are ambiguous (e.g., "5+ years" vs
  "4 years 11 months"), give the candidate the benefit of the doubt
