package app

import (
	"fmt"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/review"
)

// LensCount is what CountLenses found in the stored results of a feature.
type LensCount struct {
	// Results is how many stored results the feature has: one for each launch of a reviewer.
	Results int
	// Lenses is the number of lens rows those results record, one for each row of each launch: the number of
	// lenses the reviews of the feature ran.
	Lenses int
}

// CountLenses counts the lenses the reviews of a feature ran (own-rdd T10, S13): the rows recorded in the stored
// results of the feature under root, one for each row of each launch. A second blind launch of one head counts its
// rows, and so do the launches of a corrected candidate. A verdict of the verifier and an exemption are stored beside
// the results and are not launches of a reviewer, and a file whose name is none of the three is no result either, so
// none is read. A feature that has no stored result, or no directory, has no rows: the count is the zero LensCount
// and no error, and whether that is right for the one who asks is for them to say.
//
// Every stored result is read as Verify and Check read it (judgedStored): a file that is not one `review record`
// writes, one whose rows are missing or are not the rows of a launch, and one whose reply no longer holds to its
// binding are refused with an error that wraps review.ErrInvalidStored and names the file. A store that cannot list
// or read, and a feature that is not one safe path segment (review.CheckFeature, asked before the store is), are
// refused as well. A count that cannot be made is a refusal with the zero LensCount and never a partial number.
// Nothing is stored.
//
// It is the one place the number is made. The actuals validator calls it to hold review_lens_count of the actuals
// record to the rows we stored (tools/actuals-record-validator), and any other reader of the number calls it too.
func CountLenses(results Results, root, feature string) (LensCount, error) {
	if err := review.CheckFeature(feature); err != nil {
		return LensCount{}, err
	}
	names, err := results.Names(root, feature)
	if err != nil {
		return LensCount{}, fmt.Errorf("list %s/%s: %w", review.ResultsDir, feature, err)
	}
	var count LensCount
	for _, name := range names {
		if review.CheckResultName(name) != nil {
			continue
		}
		path := review.ResultPath(feature, name)
		file, err := results.Read(root, feature, name)
		if err != nil {
			return LensCount{}, fmt.Errorf("read %s: %w", path, err)
		}
		stored, _, err := judgedStored(file)
		if err != nil {
			return LensCount{}, fmt.Errorf("%s: %w", path, err)
		}
		count.Results++
		count.Lenses += len(stored.Rows)
	}
	return count, nil
}
