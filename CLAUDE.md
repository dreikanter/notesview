# CLAUDE.md

## Pull Requests

- Keep PR descriptions lean. Summarize the change in a few bullets; do not pad with implementation details that the diff already shows.
- Reference all related issues, PRs, and other resources when any exist. Use the `References` section with the appropriate relationship (`closes`, `relates to`, `depends on`, `blocked by`).
- Remove the `References` section entirely when there are no references — do not leave it empty.

## Search design

- Target scale: ~9K notes today, must stay performant at 100K.
- Hybrid approach:
  - Titles / paths / tags / frontmatter — DIY fuzzy subsequence scorer (fzf-style), in-memory.
  - Bodies — inverted index (token → postings with term frequency) + BM25 ranking. Rebuilt per-file on `fsnotify` change.
- Put the search layer behind an interface so Bleve can be swapped in later if phrase/proximity queries, stemming, or on-disk persistence become needed.
- Do not use naive linear substring/fuzzy scans over bodies — they fall apart around ~30K notes.
