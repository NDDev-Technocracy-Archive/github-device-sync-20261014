package workspace

import (
	"testing"

	"github.com/NDDev-Technocracy-Sandbox/github-device-sync/core/domain"
)

func consumerAnchor(id string, target string, gitmodulesName string) domain.RepositoryAnchor {
	anchor := domain.RepositoryAnchor{}
	anchor.Repository.ID = id
	anchor.Relationships = []domain.Relationship{{
		Type: "git-submodule-consumer", Target: target, GitmodulesName: gitmodulesName,
	}}
	return anchor
}

func moduleAnchor(id string) domain.RepositoryAnchor {
	anchor := domain.RepositoryAnchor{}
	anchor.Repository.ID = id
	anchor.Repository.Roles = []string{"project", "module"}
	return anchor
}

func standaloneModuleAnchor(id string) domain.RepositoryAnchor {
	anchor := moduleAnchor(id)
	anchor.Module = &domain.ModulePolicy{Development: "standalone"}
	return anchor
}

func TestEmbeddedConsumersReportsIncomingSubmoduleEdge(t *testing.T) {
	anchors := []domain.RepositoryAnchor{
		consumerAnchor("repo_super", "repo_module", "modules/ci-workflows"),
		moduleAnchor("repo_module"),
	}
	consumers := EmbeddedConsumers(anchors, "repo_module")
	if len(consumers) != 1 {
		t.Fatalf("expected one consumer, got %d", len(consumers))
	}
	if consumers[0].ConsumerID != "repo_super" ||
		consumers[0].GitmodulesName != "modules/ci-workflows" {
		t.Fatalf("unexpected consumer: %+v", consumers[0])
	}
	finding := EmbeddedOnlyFinding(moduleAnchor("repo_module"), consumers)
	if finding == nil || finding.Code != "GDS_WORKSPACE_EMBEDDED_ONLY" {
		t.Fatalf("expected GDS_WORKSPACE_EMBEDDED_ONLY, got %+v", finding)
	}
}

func TestEmbeddedConsumersIgnoresModuleRoleWithoutConsumer(t *testing.T) {
	// ADR 0018 keeps an unconsumed reusable module standalone-eligible; only an
	// actual incoming edge makes it embedded-only.
	anchors := []domain.RepositoryAnchor{moduleAnchor("repo_module")}
	if consumers := EmbeddedConsumers(anchors, "repo_module"); len(consumers) != 0 {
		t.Fatalf("expected no consumers, got %+v", consumers)
	}
	if finding := EmbeddedOnlyFinding(moduleAnchor("repo_module"), nil); finding != nil {
		t.Fatalf("expected no finding, got %+v", finding)
	}
}

func TestEmbeddedConsumersIgnoresOtherRelationshipTypes(t *testing.T) {
	anchor := domain.RepositoryAnchor{}
	anchor.Repository.ID = "repo_super"
	anchor.Relationships = []domain.Relationship{
		{Type: "workflow-module-consumer", Target: "repo_module"},
		{Type: "package-consumer", Target: "repo_module"},
	}
	if consumers := EmbeddedConsumers(
		[]domain.RepositoryAnchor{anchor}, "repo_module",
	); len(consumers) != 0 {
		t.Fatalf("expected no consumers, got %+v", consumers)
	}
}

func TestAuditLayoutRefusesStandaloneEmbeddedOnlyCheckout(t *testing.T) {
	descriptor := DeviceDescriptor{}
	descriptor.Device.ID = "device_test"
	descriptor.WorkspaceRoots = map[string]string{"nddev": "${HOME}/Developer/nddev"}
	descriptor.Materialization = MaterializationPolicy{
		DefaultMode: "absent",
		Include: []MaterializationAssignment{{
			Selector: "portfolio:organization-projects", WorkspaceRoot: "nddev", Mode: "active",
		}},
	}
	descriptor.State.Path = "${XDG_STATE_HOME}/gds"

	module := moduleAnchor("repo_module")
	module.Provider.Name = "ci-workflows"
	module.Classification.Portfolios = []string{"portfolio:organization-projects"}

	report, findings := AuditLayout(
		descriptor,
		Environment{Home: "/srv/device", XDGStateHome: "/srv/device/.local/state"},
		[]LayoutRepository{
			{Path: "/srv/device/Developer/control-plane/github-device-sync",
				Anchor: consumerAnchor("repo_super", "repo_module", "modules/ci-workflows")},
			{Path: "/srv/device/Developer/nddev/ci-workflows", Anchor: module},
		},
	)
	found := false
	for _, finding := range findings {
		if finding.Code == "GDS_WORKSPACE_EMBEDDED_ONLY" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected GDS_WORKSPACE_EMBEDDED_ONLY, got %+v", findings)
	}
	if report.Invalid == 0 {
		t.Fatalf("expected the embedded-only checkout to count as invalid")
	}
}

func TestAuditLayoutAllowsDeclaredStandaloneDevelopmentCheckout(t *testing.T) {
	// ADR 0042: a module developed in its own checkout keeps that checkout
	// beside its consumers' embedded gitlinks; both placements are legal.
	descriptor := DeviceDescriptor{}
	descriptor.Device.ID = "device_test"
	descriptor.WorkspaceRoots = map[string]string{"nddev": "${HOME}/Developer/nddev"}
	descriptor.Materialization = MaterializationPolicy{
		DefaultMode: "absent",
		Include: []MaterializationAssignment{{
			Selector: "portfolio:organization-projects", WorkspaceRoot: "nddev", Mode: "active",
		}},
	}
	descriptor.State.Path = "${XDG_STATE_HOME}/gds"

	module := standaloneModuleAnchor("repo_module")
	module.Provider.Name = "ci-workflows"
	module.Classification.Portfolios = []string{"portfolio:organization-projects"}

	report, findings := AuditLayout(
		descriptor,
		Environment{Home: "/srv/device", XDGStateHome: "/srv/device/.local/state"},
		[]LayoutRepository{
			{Path: "/srv/device/Developer/control-plane/github-device-sync",
				Anchor: consumerAnchor("repo_super", "repo_module", "modules/ci-workflows")},
			{Path: "/srv/device/Developer/nddev/ci-workflows", Anchor: module},
		},
	)
	for _, finding := range findings {
		if finding.Code == "GDS_WORKSPACE_EMBEDDED_ONLY" {
			t.Fatalf("standalone development checkout must not be embedded-only: %+v", finding)
		}
	}
	var moduleEntry *LayoutEntry
	for i := range report.Entries {
		if report.Entries[i].RepositoryID == "repo_module" {
			moduleEntry = &report.Entries[i]
		}
	}
	if moduleEntry == nil || moduleEntry.StandaloneTarget == nil {
		t.Fatalf("expected the standalone development checkout to resolve placement, got %+v", report.Entries)
	}
}

func TestSuppressDeclaredDualMaterialization(t *testing.T) {
	conflict := func(id string, first string, second string) domain.Finding {
		return domain.Finding{
			Code:     "GDS_IDENTITY_INDEX_ID_CONFLICT",
			Severity: domain.SeverityHigh,
			Evidence: map[string]any{
				"repository_id": id, "first_path": first, "second_path": second,
			},
		}
	}
	repositories := []LayoutRepository{
		{Path: "/srv/device/Developer/nddev/ci-workflows",
			Anchor: standaloneModuleAnchor("repo_module")},
		{Path: "/srv/device/Developer/control-plane/estate/modules/ci-workflows",
			SuperprojectRoot: "/srv/device/Developer/control-plane/estate",
			Anchor:           standaloneModuleAnchor("repo_module")},
		{Path: "/srv/device/Developer/nddev/alpha", Anchor: moduleAnchor("repo_alpha")},
		{Path: "/srv/device/Developer/other/alpha", Anchor: moduleAnchor("repo_alpha")},
	}
	findings := []domain.Finding{
		conflict("repo_module",
			"/srv/device/Developer/nddev/ci-workflows",
			"/srv/device/Developer/control-plane/estate/modules/ci-workflows"),
		conflict("repo_alpha",
			"/srv/device/Developer/nddev/alpha", "/srv/device/Developer/other/alpha"),
		conflict("repo_undeclared",
			"/srv/device/Developer/nddev/x", "/srv/device/Developer/other/x"),
	}
	kept := SuppressDeclaredDualMaterialization(findings, repositories)
	if len(kept) != 2 {
		t.Fatalf("expected the declared dual-materialization conflict to be suppressed, got %+v", kept)
	}
	for _, finding := range kept {
		if finding.Evidence["repository_id"] == "repo_module" {
			t.Fatalf("declared standalone development conflict must be suppressed")
		}
	}
}

func TestSuppressDeclaredDualMaterializationStaleEmbeddedPin(t *testing.T) {
	conflict := func(id string, first string, second string) domain.Finding {
		return domain.Finding{
			Code:     "GDS_IDENTITY_INDEX_ID_CONFLICT",
			Severity: domain.SeverityHigh,
			Evidence: map[string]any{
				"repository_id": id, "first_path": first, "second_path": second,
			},
		}
	}
	// The embedded consumer pin carries the module's older anchor, which has no
	// development field yet; it must not clear the standalone declaration when
	// it is inspected after the development checkout.
	repositories := []LayoutRepository{
		{Path: "/srv/device/Developer/nddev/ci-workflows",
			Anchor: standaloneModuleAnchor("repo_module")},
		{Path: "/srv/device/Developer/control-plane/estate/modules/ci-workflows",
			SuperprojectRoot: "/srv/device/Developer/control-plane/estate",
			Anchor: domain.RepositoryAnchor{
				Repository: domain.RepositoryIdentity{ID: "repo_module"},
				Module:     &domain.ModulePolicy{},
			}},
	}
	findings := []domain.Finding{
		conflict("repo_module",
			"/srv/device/Developer/nddev/ci-workflows",
			"/srv/device/Developer/control-plane/estate/modules/ci-workflows"),
	}
	if kept := SuppressDeclaredDualMaterialization(findings, repositories); len(kept) != 0 {
		t.Fatalf("stale embedded anchor must not mask the standalone declaration, got %+v", kept)
	}
}
