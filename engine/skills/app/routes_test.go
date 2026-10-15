package app

import (
	"errors"
	"io/fs"
	"reflect"
	"strings"
	"testing"

	"github.com/labdrian-ai/labdrian-sdd-overlay/engine/skills"
)

// manifestFiles is a FileReader that holds files by name and fails for the rest as a file system
// does for a file that is not there.
func manifestFiles(files map[string]string) skills.FileReader {
	return func(name string) ([]byte, error) {
		body, ok := files[name]
		if !ok {
			return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
		}
		return []byte(body), nil
	}
}

func routesInput(managedOnly bool) RoutesInput {
	return RoutesInput{
		ManifestPath: "/o/overlay.manifest",
		OverlayRoot:  "/o",
		Destinations: skills.RouteDestinations{Target: "claude", SkillsRoot: "/h/.claude/skills", AgentsRoot: "/h/.claude/agents"},
		ManagedOnly:  managedOnly,
	}
}

func TestManifestRoutesRoutesTheRowsOfTheManifestItReads(t *testing.T) {
	ports := RoutesPorts{Manifest: manifestFiles(map[string]string{
		"/o/overlay.manifest": "# rows\na/SKILL.md managed\nGADU.md custom agent\nengine/go.mod managed\n",
	})}
	got, err := ManifestRoutes(ports, routesInput(false))
	if err != nil {
		t.Fatalf("ManifestRoutes: %v", err)
	}
	want := []skills.ResolvedRoute{
		{Path: "a/SKILL.md", Route: "skill", RepoRel: "skills/a/SKILL.md", RepoSource: "/o/skills/a/SKILL.md", Dest: "/h/.claude/skills/a/SKILL.md"},
		{Path: "GADU.md", Route: "agent", RepoRel: "agents/GADU.md", RepoSource: "/o/agents/GADU.md", Dest: "/h/.claude/agents/GADU.md"},
		{Path: "engine/go.mod", Route: "non-deployable", RepoRel: "engine/go.mod"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ManifestRoutes = %+v, want %+v", got, want)
	}
}

func TestManifestRoutesKeepsOnlyTheManagedRowsWhenAsked(t *testing.T) {
	ports := RoutesPorts{Manifest: manifestFiles(map[string]string{
		"/o/overlay.manifest": "a/SKILL.md managed\nb/SKILL.md custom\n",
	})}
	got, err := ManifestRoutes(ports, routesInput(true))
	if err != nil || len(got) != 1 || got[0].Path != "a/SKILL.md" {
		t.Errorf("ManifestRoutes = %+v, %v, want the managed row alone", got, err)
	}
}

func TestManifestRoutesSaysWhyAManifestCouldNotBeRead(t *testing.T) {
	_, err := ManifestRoutes(RoutesPorts{Manifest: manifestFiles(nil)}, routesInput(false))
	var read *ManifestReadError
	if !errors.As(err, &read) || read.Path != "/o/overlay.manifest" || !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ManifestRoutes error = %v, want a ManifestReadError that names the manifest and wraps the cause", err)
	}
	if !strings.Contains(err.Error(), `reading manifest "/o/overlay.manifest"`) {
		t.Errorf("error = %q, want it to name the manifest", err)
	}
}

func TestManifestRoutesAnswersNothingForAManifestWithARowItCannotPlace(t *testing.T) {
	ports := RoutesPorts{Manifest: manifestFiles(map[string]string{
		"/o/overlay.manifest": "a/SKILL.md managed\nlongterm-mem/x.go custom\n",
	})}
	got, err := ManifestRoutes(ports, routesInput(false))
	var unrouted *skills.UnroutedRowError
	if !errors.As(err, &unrouted) || got != nil {
		t.Errorf("ManifestRoutes = %+v, %v, want the refusal of the row and no route at all", got, err)
	}
}
