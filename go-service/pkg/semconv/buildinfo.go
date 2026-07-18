package semconv

import (
	"runtime"

	"go.opentelemetry.io/otel/attribute"
	otelsemconv "go.opentelemetry.io/otel/semconv/v1.41.0"
)

// Build information. These variables are populated at link time via -ldflags
// -X, driven by git metadata (see the Makefile / .goreleaser.yaml). They default
// to "dev"/"unknown" for local `go run`/`go build` invocations.
var (
	// ServiceName is the OTel service.name for this microservice.
	ServiceName = "student-report-service"
	// Version is the released version (git tag/describe) or "dev".
	Version = "dev"
	// Revision is the git commit SHA the binary was built from.
	Revision = "unknown"
	// Tag is the git tag the binary was built from, if any.
	Tag = ""
	// Repository is the source repository URL.
	Repository = "https://github.com/gaarutyunov/skill-test"
	// Date is the RFC3339 build timestamp.
	Date = "unknown"
)

// BuildInfo is a snapshot of the compiled-in build metadata.
type BuildInfo struct {
	ServiceName string
	Version     string
	Revision    string
	Tag         string
	Repository  string
	Date        string
	GoVersion   string
}

// Build returns the compiled-in build information.
func Build() BuildInfo {
	return BuildInfo{
		ServiceName: ServiceName,
		Version:     Version,
		Revision:    Revision,
		Tag:         Tag,
		Repository:  Repository,
		Date:        Date,
		GoVersion:   runtime.Version(),
	}
}

// ResourceAttributes returns the OTel resource attributes describing this build.
// Service, VCS, and process-runtime metadata use the OFFICIAL OpenTelemetry
// semantic conventions. Only build.date is project-specific (generated into
// this package from the Weaver registry), because no official attribute exists
// for a build timestamp.
func (b BuildInfo) ResourceAttributes() []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		otelsemconv.ServiceName(b.ServiceName),
		otelsemconv.ServiceVersion(b.Version),
		otelsemconv.VCSRepositoryURLFull(b.Repository),
		otelsemconv.VCSRefHeadRevision(b.Revision),
		otelsemconv.ProcessRuntimeName("go"),
		otelsemconv.ProcessRuntimeVersion(b.GoVersion),
		otelsemconv.ProcessRuntimeDescription("Go runtime"),
		// Project-specific: no official build-timestamp convention exists.
		BuildDate(b.Date),
	}
	if b.Tag != "" {
		attrs = append(attrs, otelsemconv.VCSRefHeadName(b.Tag))
	}
	return attrs
}
