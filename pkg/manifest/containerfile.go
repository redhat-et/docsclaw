package manifest

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"text/template"

	"github.com/redhat-et/docsclaw/pkg/catalog"
)

var containerfileTmpl = template.Must(template.New("containerfile").Parse(`FROM {{ .GoBuilderImage }} AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /docsclaw ./cmd/docsclaw

FROM {{ .BaseImage }}

LABEL io.docsclaw.tools/installed="{{ .InstalledCSV }}"
LABEL io.docsclaw.tools/tier="{{ .HighestTier }}"
LABEL io.docsclaw.tools/risk-score="{{ .RiskScore }}"
LABEL io.docsclaw.tools/agent-name="{{ .AgentName }}"

USER root
{{ if .HasToolBuilder -}}
# Adding tools to the minimal hardened image expands its attack surface.
# Only add what is strictly necessary for runtime operation.
# Review each addition with your security team.
RUN --mount=type=bind,from={{ .ToolBuilderImage }},target=/builder \
    LD_LIBRARY_PATH=/builder/lib64:/builder/usr/lib64 \
    RPM_CONFIGDIR=/builder/usr/lib/rpm \
    /builder/usr/bin/dnf install -y \
    --installroot=/ \
    --setopt=reposdir=/builder/etc/yum.repos.d \
    --setopt=install_weak_deps=False \
    --setopt=tsflags=nodocs \
    {{ .PackageList }}
{{ end -}}
RUN mkdir -p /etc/docsclaw
USER 65532

WORKDIR /app
COPY --from=builder /docsclaw /app/docsclaw
COPY tools.json /etc/docsclaw/tools.json

EXPOSE 8000

ENTRYPOINT ["/app/docsclaw"]
CMD ["serve"]
`))

type containerfileData struct {
	GoBuilderImage  string
	BaseImage       string
	ToolBuilderImage string
	HasToolBuilder  bool
	AgentName       string
	InstalledCSV    string
	HighestTier     string
	RiskScore       int
	PackageList     string
}

func GenerateContainerfile(m *AgentManifest, cat *catalog.ToolCatalog) (string, error) {
	allTools := MergeWithCore(m.Spec.Tools, cat)
	sort.Strings(allTools)

	pkgs := cat.PackageNames(allTools, "dnf")
	sort.Strings(pkgs)

	goBuilder := m.Spec.Base.GoBuilder
	if goBuilder == "" {
		goBuilder = "registry.access.redhat.com/hi/go:latest"
	}

	data := containerfileData{
		GoBuilderImage:  goBuilder,
		BaseImage:       m.Spec.Base.Image,
		ToolBuilderImage: m.Spec.Base.ToolBuilder,
		HasToolBuilder:  m.Spec.Base.ToolBuilder != "",
		AgentName:       m.Metadata.Name,
		InstalledCSV:    strings.Join(allTools, ","),
		HighestTier:     cat.HighestTier(allTools),
		RiskScore:       cat.MaxRiskScore(allTools),
		PackageList:     strings.Join(pkgs, " "),
	}

	var buf bytes.Buffer
	if err := containerfileTmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("render containerfile: %w", err)
	}
	return buf.String(), nil
}

func MergeWithCore(tools []string, cat *catalog.ToolCatalog) []string {
	seen := make(map[string]bool)
	var merged []string
	for _, name := range cat.CoreTools() {
		if !seen[name] {
			seen[name] = true
			merged = append(merged, name)
		}
	}
	for _, name := range tools {
		if !seen[name] {
			seen[name] = true
			merged = append(merged, name)
		}
	}
	return merged
}
