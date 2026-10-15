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
	if !reflect.DeepEqual(got.Routes, want) || got.ManifestMissing {
		t.Errorf("ManifestRoutes = %+v, want the routes %+v of a manifest that is there", got, want)
	}
}

func TestManifestRoutesKeepsOnlyTheManagedRowsWhenAsked(t *testing.T) {
	ports := RoutesPorts{Manifest: manifestFiles(map[string]string{
		"/o/overlay.manifest": "a/SKILL.md managed\nb/SKILL.md custom\nc/SKILL.md\n",
	})}
	got, err := ManifestRoutes(ports, routesInput(true))
	if err != nil || len(got.Routes) != 1 || got.Routes[0].Path != "a/SKILL.md" {
		t.Errorf("ManifestRoutes = %+v, %v, want the managed row alone", got, err)
	}
}

func TestManifestRoutesReadsAManifestThatIsNotThereAsOneWithNoRow(t *testing.T) {
	got, err := ManifestRoutes(RoutesPorts{Manifest: manifestFiles(nil)}, routesInput(false))
	if err != nil || len(got.Routes) != 0 || !got.ManifestMissing {
		t.Errorf("ManifestRoutes = %+v, %v, want no row, no error, and the manifest said to be missing", got, err)
	}
}

func TestManifestRoutesStillRefusesAnUnknownTargetWhenTheManifestIsNotThere(t *testing.T) {
	in := routesInput(false)
	in.Destinations.Target = "windsurf"
	_, err := ManifestRoutes(RoutesPorts{Manifest: manifestFiles(nil)}, in)
	var unknown *skills.UnknownTargetError
	if !errors.As(err, &unknown) {
		t.Errorf("ManifestRoutes error = %v, want an UnknownTargetError: a manifest that is not there does not excuse the question", err)
	}
}

func TestManifestRoutesSaysWhyAManifestThatIsThereCouldNotBeRead(t *testing.T) {
	denied := &fs.PathError{Op: "open", Path: "/o/overlay.manifest", Err: fs.ErrPermission}
	ports := RoutesPorts{Manifest: func(string) ([]byte, error) { return nil, denied }}
	_, err := ManifestRoutes(ports, routesInput(false))
	var read *ManifestReadError
	if !errors.As(err, &read) || read.Path != "/o/overlay.manifest" || !errors.Is(err, fs.ErrPermission) {
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
	if !errors.As(err, &unrouted) || got.Routes != nil {
		t.Errorf("ManifestRoutes = %+v, %v, want the refusal of the row and no route at all", got, err)
	}
}
