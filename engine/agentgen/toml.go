package agentgen

// The two forms of TOML string the Codex agent files use. The engine has no TOML library and needs
// no more than this: a basic string on one line and a multi-line basic string. Both refuse what TOML
// does not allow in a string, instead of writing a file that Codex would refuse.

import (
	"fmt"
	"strings"
)

// tomlString is s as a basic string on one line. A backslash and a quotation mark are escaped; a
// control character other than a tab is an error.
func tomlString(s string) (string, error) {
	if err := refuseControlCharacters(s, false); err != nil {
		return "", err
	}
	return `"` + escapeBasic(s) + `"`, nil
}

// tomlMultiline is s as a multi-line basic string: the opening delimiter and its newline, s, and the
// closing delimiter. A backslash is escaped, and so is the third quotation mark of any run of three, so
// that no run of quotation marks in s closes the string early. A control character other than a tab
// and a newline is an error.
func tomlMultiline(s string) (string, error) {
	if err := refuseControlCharacters(s, true); err != nil {
		return "", err
	}
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"""`, `""\"`)
	return "\"\"\"\n" + s + `"""`, nil
}

// codexKeys are the four keys of a Codex agent file, in the order codexFile writes them.
var codexKeys = []string{"name", "description", "developer_instructions", "model_reasoning_effort"}

// CodexInstructions is the developer_instructions of a Codex agent file, the exact inverse of codexFile and
// tomlMultiline: the file must be the four lines `name = "..."`, `description = "..."`,
// `developer_instructions = """` and its newline up to the closing delimiter, and `model_reasoning_effort = "..."`, in that
// order and nothing else. In the body `\\` is a backslash and `""\"` is three quotation marks; any other backslash
// sequence is an error, as is anything that is not the generator's shape. The error says what is not.
func CodexInstructions(data []byte) (string, error) {
	rest := string(data)
	var instructions string
	for _, key := range codexKeys {
		prefix := key + " = "
		if !strings.HasPrefix(rest, prefix) {
			return "", fmt.Errorf("not an agent file the generator wrote: expected the key %q next, in the order %s", key, strings.Join(codexKeys, ", "))
		}
		rest = rest[len(prefix):]
		var err error
		if key == "developer_instructions" {
			instructions, rest, err = readMultiline(rest)
		} else {
			rest, err = skipBasic(rest)
		}
		if err != nil {
			return "", fmt.Errorf("not an agent file the generator wrote: %s: %w", key, err)
		}
		if !strings.HasPrefix(rest, "\n") {
			return "", fmt.Errorf("not an agent file the generator wrote: %s is not the end of its line", key)
		}
		rest = rest[1:]
	}
	if rest != "" {
		return "", fmt.Errorf("not an agent file the generator wrote: something follows %s", codexKeys[len(codexKeys)-1])
	}
	return instructions, nil
}

// skipBasic reads a basic string on one line from the start of s and returns what follows its closing quotation mark.
func skipBasic(s string) (rest string, err error) {
	if !strings.HasPrefix(s, `"`) {
		return "", fmt.Errorf("not a basic string")
	}
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			if i+1 >= len(s) || (s[i+1] != '\\' && s[i+1] != '"') {
				return "", fmt.Errorf("unknown escape at byte %d", i)
			}
			i++
		case '"':
			return s[i+1:], nil
		case '\n':
			return "", fmt.Errorf("unterminated basic string")
		}
	}
	return "", fmt.Errorf("unterminated basic string")
}

// readMultiline reads a multi-line basic string from the start of s, which opens with its delimiter and a newline, and
// returns its decoded body and what follows the closing delimiter.
func readMultiline(s string) (body, rest string, err error) {
	if !strings.HasPrefix(s, "\"\"\"\n") {
		return "", "", fmt.Errorf("not a multi-line string opened by three quotation marks and a newline")
	}
	var b strings.Builder
	for i := 4; i < len(s); {
		switch s[i] {
		case '\\':
			if i+1 >= len(s) || (s[i+1] != '\\' && s[i+1] != '"') {
				return "", "", fmt.Errorf("unknown escape at byte %d", i)
			}
			b.WriteByte(s[i+1])
			i += 2
		case '"':
			n := 0
			for i+n < len(s) && s[i+n] == '"' {
				n++
			}
			switch {
			case n < 3:
				b.WriteString(strings.Repeat(`"`, n))
				i += n
			case n <= 5:
				b.WriteString(strings.Repeat(`"`, n-3))
				return b.String(), s[i+n:], nil
			default:
				return "", "", fmt.Errorf("a run of %d quotation marks at byte %d", n, i)
			}
		default:
			b.WriteByte(s[i])
			i++
		}
	}
	return "", "", fmt.Errorf("unterminated multi-line string")
}

func escapeBasic(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, `"`, `\"`)
}

// refuseControlCharacters fails for the characters TOML does not allow in a basic string: the
// control characters other than a tab, and delete. A newline is allowed in a multi-line string only; a
// carriage return is not allowed at all (TOML takes it only as the first half of a line ending, and
// the files of the generator end their lines with a newline).
func refuseControlCharacters(s string, multiline bool) error {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\t' || (c == '\n' && multiline) {
			continue
		}
		if c < 0x20 || c == 0x7f {
			return fmt.Errorf("control character U+%04X at byte %d cannot be written in a TOML string", c, i)
		}
	}
	return nil
}
