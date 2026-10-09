package projection

import "testing"

// The digest of the path as a program that knows nothing of this package computes it:
//
//	printf '%s' '/srv/repo/.git' | sha256sum
const keyOfSrvRepoGit = "4b45de8d0544bbd0b50cad974a6064025da0d6473df655f4569e46e555d28da4"

func TestRepoKeyOfIsTheSHA256OfTheCleanedPath(t *testing.T) {
	if got := RepoKeyOf("/srv/repo/.git"); got != keyOfSrvRepoGit {
		t.Fatalf("RepoKeyOf(/srv/repo/.git) = %q, want %q", got, keyOfSrvRepoGit)
	}
}

func TestRepoKeyOfCleansThePathBeforeHashingIt(t *testing.T) {
	for _, spelled := range []string{"/srv//repo/.git", "/srv/repo/./.git", "/srv/repo/.git/", "/srv/other/../repo/.git"} {
		if got := RepoKeyOf(spelled); got != keyOfSrvRepoGit {
			t.Errorf("RepoKeyOf(%q) = %q, want the key of /srv/repo/.git, %q", spelled, got, keyOfSrvRepoGit)
		}
	}
}

func TestRepoKeyOfTellsPathsApart(t *testing.T) {
	if RepoKeyOf("/srv/repo/.git") == RepoKeyOf("/srv/repo2/.git") {
		t.Fatal("two repositories have the same key")
	}
}

func TestRepoKeyOfIsAKeyTheBindingAccepts(t *testing.T) {
	for _, path := range []string{"/srv/repo/.git", "/", "relative/.git", ""} {
		if err := ValidateRepoKey(RepoKeyOf(path)); err != nil {
			t.Errorf("ValidateRepoKey(RepoKeyOf(%q)) = %v, want nil", path, err)
		}
	}
}
