---
name: url-summary
description: Fetch a URL and produce a structured summary
---

# URL summary skill

When asked to summarize content from a URL:

1. Use the `web_fetch` tool to retrieve the page content
2. If the content is HTML, extract the main text (ignore nav, ads)
3. Produce a summary with:
   - **Title**: the page or document title
   - **Key points**: 3-5 bullet points
   - **Notable details**: dates, numbers, names mentioned
4. Use `write_file` to save the summary if the user requests it
