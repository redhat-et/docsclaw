package manifest

import (
	"strings"
	"testing"

	"github.com/redhat-et/docsclaw/pkg/catalog"
)

func TestGenerateContainerfile_HardenedImage(t *testing.T) {
	m := &AgentManifest{
		Metadata: ManifestMeta{Name: "test-agent"},
		Spec: ManifestSpec{
			Base: BaseImage{
				Image:   "registry.access.redhat.com/hi/core-runtime:latest",
				Builder: "registry.access.redhat.com/hi/core-runtime:latest-builder",
			},
			Tools: []string{"curl", "jq", "git"},
		},
	}
	cat, _ := catalog.LoadDefault()

	out, err := GenerateContainerfile(m, cat)
	if err != nil {
		t.Fatalf("GenerateContainerfile() error: %v", err)
	}

	checks := []string{
		"FROM registry.access.redhat.com/hi/core-runtime:latest",
		"io.docsclaw.tools/installed",
		"curl,git,jq",
		"--mount=type=bind,from=registry.access.redhat.com/hi/core-runtime:latest-builder",
		"/builder/usr/bin/dnf install -y",
		"curl git jq",
		"USER 65532",
		"COPY docsclaw /app/docsclaw",
		"COPY tools.json /etc/docsclaw/tools.json",
		"mkdir -p /etc/docsclaw",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestGenerateContainerfile_Labels(t *testing.T) {
	m := &AgentManifest{
		Metadata: ManifestMeta{Name: "my-agent"},
		Spec: ManifestSpec{
			Base: BaseImage{
				Image:   "registry.access.redhat.com/hi/core-runtime:latest",
				Builder: "registry.access.redhat.com/hi/core-runtime:latest-builder",
			},
			Tools: []string{"curl", "jq", "python3"},
		},
	}
	cat, _ := catalog.LoadDefault()

	out, err := GenerateContainerfile(m, cat)
	if err != nil {
		t.Fatalf("GenerateContainerfile() error: %v", err)
	}

	if !strings.Contains(out, `io.docsclaw.tools/tier="runtime"`) {
		t.Error("missing tier label for runtime")
	}
	if !strings.Contains(out, `io.docsclaw.tools/agent-name="my-agent"`) {
		t.Error("missing agent-name label")
	}
}

func TestGenerateContainerfile_CoreOnlyNoBuilder(t *testing.T) {
	m := &AgentManifest{
		Metadata: ManifestMeta{Name: "minimal"},
		Spec: ManifestSpec{
			Base: BaseImage{
				Image: "registry.access.redhat.com/hi/core-runtime:latest",
			},
			Tools: []string{"curl", "jq"},
		},
	}
	cat, _ := catalog.LoadDefault()

	out, err := GenerateContainerfile(m, cat)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if !strings.Contains(out, "FROM registry.access.redhat.com/hi/core-runtime:latest") {
		t.Error("missing FROM line")
	}
	if !strings.Contains(out, "COPY tools.json /etc/docsclaw/tools.json") {
		t.Error("missing tools.json COPY even without builder")
	}
	if !strings.Contains(out, "mkdir -p /etc/docsclaw") {
		t.Error("missing /etc/docsclaw mkdir even without builder")
	}
}
