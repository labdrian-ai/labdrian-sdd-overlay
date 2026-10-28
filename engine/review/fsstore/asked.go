package fsstore

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/statestore"
)

// storeName is how the questions store names itself in an error, and errUnsupported is what a platform without a
// no-follow open is refused with.
const storeName = "review stop-hook store"

var errUnsupported = errors.New(storeName + ": unsupported platform")

// askedMarker is what a marker holds: nothing a reader needs, because the marker is its name.
var askedMarker = []byte("asked\n")

// maxKeyLen bounds the key of a question, which is <base12>-<head12> or a fixed word.
const maxKeyLen = 64

// Asked is the adapter of the Asked port of review/app: the questions the stop hook has put, as one private marker for
// each session and question, <state home>/labdrian/review-stop/<sha256 of the session>/<key>, kept with the other
// stores of the user and outside every working tree (own-rdd T6a, S6). The session is hashed because it is a name
// Claude Code gives, not a path, and no name of a session reaches the file system. It is built over the state home
// and reads no variable of the environment. The directories are made 0700 and refused when one is a link
// (statestore.EnsureDirs); a marker is published once, never replaced, and recording one again changes nothing
// (statestore.Publish).
type Asked struct{ home string }

// NewAsked is the store over the state home stateHome, which must be an absolute path, on a platform that has a
// no-follow open (linux and darwin).
func NewAsked(stateHome string) (Asked, error) {
	if err := statestore.RequireStore(storeName, errUnsupported, stateHome); err != nil {
		return Asked{}, err
	}
	return Asked{home: stateHome}, nil
}

// Has says whether the question named key was put in the session. It makes nothing: a store nobody wrote to is absent,
// not broken.
func (a Asked) Has(session, key string) (bool, error) {
	if err := checkKey(key); err != nil {
		return false, err
	}
	parts := a.directory(session)
	if err := statestore.CheckDirs(parts); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("%s: %w", storeName, err)
	}
	_, err := statestore.ReadFile(markerPath(parts, key), int64(len(askedMarker))+1)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	}
	return false, fmt.Errorf("%s: %w", storeName, err)
}

// Add records that the question named key was put in the session, making the directories that are not there.
func (a Asked) Add(session, key string) error {
	if err := checkKey(key); err != nil {
		return err
	}
	parts := a.directory(session)
	if err := statestore.EnsureDirs(parts); err != nil {
		return fmt.Errorf("%s: %w", storeName, err)
	}
	if err := statestore.Publish(markerPath(parts, key), askedMarker); err != nil {
		return fmt.Errorf("%s: %w", storeName, err)
	}
	return nil
}

// directory is the state home followed by the components of the directory of the session.
func (a Asked) directory(session string) []string {
	sum := sha256.Sum256([]byte(session))
	return []string{a.home, "labdrian", "review-stop", hex.EncodeToString(sum[:])}
}

func markerPath(directory []string, key string) string {
	return filepath.Join(filepath.Join(directory...), key)
}

// checkKey requires a key to be one plain name of lowercase letters, digits and dashes, starting with a letter or a
// digit, so that no key a caller composes leads out of the directory of the session.
func checkKey(key string) error {
	ok := key != "" && len(key) <= maxKeyLen && key[0] != '-'
	for i := 0; ok && i < len(key); i++ {
		c := key[i]
		ok = c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-'
	}
	if !ok {
		return fmt.Errorf("%s: %q is not a key of lowercase letters, digits and dashes of at most %d bytes", storeName, key, maxKeyLen)
	}
	return nil
}
