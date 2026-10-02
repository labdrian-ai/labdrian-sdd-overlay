package shaper

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// cleanContainedRelPath applies the lexical half of the contained-read rules
// shared by BindGoal and LoadHandoff: worktreeRoot must be a non-empty
// absolute path and relPath a non-empty relative path that neither cleans to
// the root itself nor traverses above it. It returns the cleaned relPath.
// Errors carry no operation prefix; callers add their own.
//
// The rules are about how a path is spelled, so they hold whatever file system
// answers the read. Whether the file that is read lies inside the root is not
// decided here: it is the ContainedSource's to prove.
func cleanContainedRelPath(worktreeRoot, argName, relPath string) (string, error) {
	if worktreeRoot == "" {
		return "", fmt.Errorf("worktreeRoot must not be empty")
	}
	if !filepath.IsAbs(worktreeRoot) {
		return "", fmt.Errorf("worktreeRoot must be absolute, got %q", worktreeRoot)
	}
	if relPath == "" {
		return "", fmt.Errorf("%s must not be empty", argName)
	}
	if filepath.IsAbs(relPath) {
		return "", fmt.Errorf("%s must be relative, got %q", argName, relPath)
	}
	cleaned := filepath.Clean(relPath)
	if cleaned == "." {
		return "", fmt.Errorf("%s must not resolve to the worktree root itself, got %q", argName, relPath)
	}
	for _, part := range strings.Split(cleaned, string(filepath.Separator)) {
		if part == ".." {
			return "", fmt.Errorf("%s must not traverse outside the worktree root, got %q", argName, relPath)
		}
	}
	return cleaned, nil
}

// errNoContainedSource is what every read reports when it is given no
// ContainedSource: a read with nowhere to read from is refused, never guessed.
var errNoContainedSource = errors.New("no contained source to read from")

// readSource is the read every entry point shares: the path must pass the
// lexical rules (cleanContainedRelPath, with argName naming the argument in
// its errors) and then the ContainedSource reads it, with label naming the
// source in its errors. It returns the cleaned path and the exact bytes read.
// Errors carry no operation prefix; callers add their own.
func readSource(src ContainedSource, worktreeRoot, argName, relPath, label string) (string, []byte, error) {
	cleaned, err := cleanContainedRelPath(worktreeRoot, argName, relPath)
	if err != nil {
		return "", nil, err
	}
	if src == nil {
		return "", nil, errNoContainedSource
	}
	data, err := src.ReadContained(worktreeRoot, cleaned, label)
	if err != nil {
		return "", nil, err
	}
	return cleaned, data, nil
}

// ReadContainedSource reads relPath strictly inside worktreeRoot under the
// same containment rules as LoadHandoff and BindGoal, without parsing it. It
// returns the cleaned root-relative path and the exact bytes read. A caller
// uses it to hand bytes that failed a strict parse to Evaluate, which then
// reports them as a blocker instead of an I/O failure.
func ReadContainedSource(src ContainedSource, worktreeRoot, relPath string) (string, []byte, error) {
	cleaned, data, err := readSource(src, worktreeRoot, "path", relPath, "source")
	if err != nil {
		return "", nil, fmt.Errorf("read source: %w", err)
	}
	return cleaned, data, nil
}

// SourceSHA256 is the lowercase hex SHA-256 of source bytes. It detects
// drift only and is not a signature.
func SourceSHA256(data []byte) string {
	return sha256Hex(data)
}

// sha256Hex returns the lowercase hex SHA-256 of data.
func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
