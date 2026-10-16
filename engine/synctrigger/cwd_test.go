package synctrigger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAbsoluteCwd(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, in, want string }{
		{"an absolute path stays as it is", "/some/dir", "/some/dir"},
		{"an absolute path is not cleaned", "/some//dir/../dir", "/some//dir/../dir"},
		{"an empty path stays empty: it never inherits the caller's directory", "", ""},
		{"a relative path is taken from the working directory", "rel/dir", filepath.Join(wd, "rel", "dir")},
		{"a dot is the working directory", ".", wd},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := absoluteCwd(tc.in); got != tc.want {
				t.Errorf("absoluteCwd(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestReportUsageNamesTheEventAndTheCwdQuotedAsTheCallerGaveThem(t *testing.T) {
	var b strings.Builder

	reportUsage(&b, "bo\"gus", "/a b\n")

	if want := "sync-trigger: error:usage event=\"bo\\\"gus\" cwd=\"/a b\\n\"\n"; b.String() != want {
		t.Errorf("reportUsage wrote %q, want %q", b.String(), want)
	}
}

func TestReportFailureNamesTheKindAndWhatFailed(t *testing.T) {
	var b strings.Builder

	reportFailure(&b, kindSpawn, os.ErrPermission)

	if want := "sync-trigger: error:spawn: permission denied\n"; b.String() != want {
		t.Errorf("reportFailure wrote %q, want %q", b.String(), want)
	}
}
