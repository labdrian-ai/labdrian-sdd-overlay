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
