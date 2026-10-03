# Changelog

## Unreleased

### Changed

- Contract frontmatter lists must be inline lists, `[a, b]`. The shapes that were tolerated (no brackets, an unclosed bracket, text after the bracket, an empty value) are now refused with a message naming the key, and quoted items now match.
