package app

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

func TestListSkillsSortsByIdAndCarriesWhatListTells(t *testing.T) {
	repo := registries{"r.yaml": registryOf(
		entry("zeta", "custom", "claude"),
		entry("alpha", "core", "claude", "pi"),
		entry("mid", "custom", "codex"),
	)}
	got, err := ListSkills(repo, ListInput{RegistryPath: "r.yaml"})
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}
	want := []ListedSkill{
		{ID: "alpha", SourceType: "core", UpdateStrategy: "vendor-merge", Targets: []string{"claude", "pi"}},
		{ID: "mid", SourceType: "custom", UpdateStrategy: "overlay-only", Targets: []string{"codex"}},
		{ID: "zeta", SourceType: "custom", UpdateStrategy: "overlay-only", Targets: []string{"claude"}},
	}
	if !reflect.DeepEqual(got.Skills, want) {
		t.Errorf("Skills = %+v, want %+v", got.Skills, want)
	}
}

func TestListSkillsDoesNotReorderTheRegistryItRead(t *testing.T) {
	reg := registryOf(entry("b", "custom", "claude"), entry("a", "custom", "claude"))
	if _, err := ListSkills(registries{"r": reg}, ListInput{RegistryPath: "r"}); err != nil {
		t.Fatal(err)
	}
	if reg.Skills[0].ID != "b" {
		t.Errorf("the registry the repository holds was sorted in place: first entry is %q", reg.Skills[0].ID)
	}
}

func TestListSkillsOfAnEmptyRegistryListsNothing(t *testing.T) {
	got, err := ListSkills(registries{"r": registryOf()}, ListInput{RegistryPath: "r"})
	if err != nil || len(got.Skills) != 0 {
		t.Errorf("ListSkills = %+v, %v, want no skills and no error", got, err)
	}
}

func TestListSkillsSaysWhatTheReaderLeftOut(t *testing.T) {
	reg := registryOf(entry("a", "custom", "claude"))
	reg.Unread = []string{"line 4: field \"extra\" is not read"}
	got, err := ListSkills(registries{"r": reg}, ListInput{RegistryPath: "r"})
	if err != nil {
		t.Fatal(err)
	}
	if got.UnreadWarning == "" || got.UnreadWarning != reg.UnreadWarning() {
		t.Errorf("UnreadWarning = %q, want the domain's wording %q", got.UnreadWarning, reg.UnreadWarning())
	}
}

func TestListSkillsTellsAStoreThatCannotBeReadFromARegistryThatIsNotUsable(t *testing.T) {
	_, err := ListSkills(registries{}, ListInput{RegistryPath: "absent.yaml"})
	var refusal *RegistryError
	if !errors.As(err, &refusal) || !refusal.Unreadable() || refusal.Path != "absent.yaml" {
		t.Fatalf("err = %v, want an unreadable RegistryError naming absent.yaml", err)
	}
	if !strings.Contains(err.Error(), "no such file") {
		t.Errorf("err = %q, want the words of the store", err)
	}

	bad := registryOf(entry("a", "custom"))
	_, err = ListSkills(registries{"bad": bad}, ListInput{RegistryPath: "bad"})
	if !errors.As(err, &refusal) || refusal.Unreadable() {
		t.Fatalf("err = %v, want a RegistryError that is not unreadable (the registry has no targets)", err)
	}
}

func TestListSkillsRefusesWhenNoRepositoryIsWired(t *testing.T) {
	_, err := ListSkills(nil, ListInput{RegistryPath: "r"})
	var refusal *RegistryError
	if !errors.As(err, &refusal) {
		t.Fatalf("err = %v, want a RegistryError, not a panic", err)
	}
}

func TestRegistryStatusCountsBySource(t *testing.T) {
	repo := registries{"r": registryOf(
		entry("a", "core", "claude"),
		entry("b", "custom", "claude"),
		entry("c", "custom", "claude"),
		skillsExternal("d"),
	)}
	got, err := RegistryStatus(repo, StatusInput{RegistryPath: "r"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 4 || got.Core != 1 || got.Custom != 2 {
		t.Errorf("status = %+v, want 4 in all, 1 core, 2 custom (an external one is counted in the total only)", got)
	}
}

func TestRegistryStatusOfAnUnusableRegistryIsARegistryError(t *testing.T) {
	_, err := RegistryStatus(registries{}, StatusInput{RegistryPath: "absent"})
	var refusal *RegistryError
	if !errors.As(err, &refusal) || !refusal.Unreadable() {
		t.Errorf("err = %v, want an unreadable RegistryError", err)
	}
}

func skillsExternal(id string) skills.Entry {
	e := entry(id, "external", "claude")
	e.Source.Repo = "https://example.test/org/" + id
	e.Source.Ref = "v1"
	return e
}
