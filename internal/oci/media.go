// Package oci provides OCI artifact operations for skill distribution.
package oci

const (
	ArtifactType         = "application/vnd.agentskills.skill.v1"
	ConfigMediaType      = "application/vnd.agentskills.skill.config.v1+json"
	ImageConfigMediaType = "application/vnd.oci.image.config.v1+json"
	CardMediaType        = "application/vnd.docsclaw.skill.card.v1+yaml"
	ContentMediaType     = "application/vnd.agentskills.skill.content.v1.tar+gzip"
	FileMediaType        = "application/octet-stream"
)

const (
	AnnotationTitle           = "org.opencontainers.image.title"
	AnnotationVersion         = "org.opencontainers.image.version"
	AnnotationDescription     = "org.opencontainers.image.description"
	AnnotationLicenses        = "org.opencontainers.image.licenses"
	AnnotationCreated         = "org.opencontainers.image.created"
	AnnotationSkillName       = "io.agentskills.skill.name"
	AnnotationResourcesMemory = "io.docsclaw.skill.resources.memory"
	AnnotationResourcesCPU    = "io.docsclaw.skill.resources.cpu"
	AnnotationToolsRequired   = "io.docsclaw.skill.tools.required"
)
