package fsstore

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review/app"
)

// Features says whether the working tree under root keeps features, which it does when odd/tasks or
// odd/review-results is a directory in it, and lists the features that have a results directory, sorted (own-rdd T6a,
// S6). It makes nothing and follows no link: a link or a file where odd, odd/tasks or odd/review-results should be a
// directory is ErrNotADirectory, and a link, a file or a name that is not a feature (review.CheckFeature) inside
// odd/review-results is not a feature. It works by path on every platform: it only reads names, and whatever is done
// with a name afterwards goes through Names, which walks to the directory as it always does.
func (s Store) Features(root string) (app.Features, error) {
	odd, err := plainDirectory(filepath.Join(root, "odd"))
	if err != nil || !odd {
		return app.Features{}, err
	}
	tasks, err := plainDirectory(filepath.Join(root, "odd", "tasks"))
	if err != nil {
		return app.Features{}, err
	}
	results := filepath.Join(root, review.ResultsDir)
	kept, err := plainDirectory(results)
	if err != nil {
		return app.Features{}, err
	}
	features := app.Features{Tracked: tasks || kept}
	if !kept {
		return features, nil
	}
	entries, err := os.ReadDir(results)
	if err != nil {
		return app.Features{}, fmt.Errorf("list %s: %w", results, err)
	}
	for _, entry := range entries {
		if entry.IsDir() && review.CheckFeature(entry.Name()) == nil {
			features.Names = append(features.Names, entry.Name())
		}
	}
	return features, nil
}

// plainDirectory says whether a directory is at path. Nothing there is false, and a link or a file there is
// ErrNotADirectory.
func plainDirectory(path string) (bool, error) {
	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("inspect %s: %w", path, err)
	case !info.IsDir():
		return false, fmt.Errorf("%w: %s", ErrNotADirectory, path)
	}
	return true, nil
}
