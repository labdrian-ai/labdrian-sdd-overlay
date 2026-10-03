# Changelog

## Unreleased

### Changed

- The skills registry reader dispatches on the file's `version`: a version it does not know is refused first, naming it. A key it does not know no longer refuses the file: it is left out and said on stderr, the verbs that only read go on with the rest, and `skills add` and `skills remove` refuse such a registry because writing it back would drop the key. A field that install, approval or projection depend on (`id`, `path`, `source`, `source.type`, `install`, `install.defaultScope`, `install.allowedProjects`, `install.targets`) is refused, naming the field and the line, when it is not in the shape the reader reads; before, a value on the line of `source`, `install` or a list was ignored without a word.
- Contract frontmatter lists must be inline lists, `[a, b]`. The shapes that were tolerated (no brackets, an unclosed bracket, text after the bracket, an empty value) are now refused with a message naming the key, and quoted items now match.
