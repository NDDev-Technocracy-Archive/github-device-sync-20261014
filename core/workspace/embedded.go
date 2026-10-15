package workspace

import (
	"path/filepath"
	"sort"

	"github.com/NDDev-Technocracy-Sandbox/github-device-sync/core/domain"
)

// EmbeddedConsumer names one repository that consumes another repository as a
// Git submodule gitlink.
type EmbeddedConsumer struct {
	ConsumerID     string `json:"consumer_id"`
	GitmodulesName string `json:"gitmodules_name,omitempty"`
}

// EmbeddedConsumers returns every observed anchor that declares repositoryID as
// the target of a `git-submodule-consumer` relationship.
//
// Membership is decided by the incoming edge alone, never by the dependency's
// own `module` role: a reusable module with no consumer in this estate is
// legitimately standalone (ADR 0018), while a consumed one is not (ADR 0027).
func EmbeddedConsumers(
	anchors []domain.RepositoryAnchor,
	repositoryID string,
) []EmbeddedConsumer {
	consumers := []EmbeddedConsumer{}
	if repositoryID == "" {
		return consumers
	}
	seen := map[string]bool{}
	for _, anchor := range anchors {
		if anchor.Repository.ID == repositoryID {
			continue
		}
		for _, relationship := range matchingSubmoduleRelations(anchor, repositoryID) {
			key := anchor.Repository.ID + "\x00" + relationship.GitmodulesName
			if seen[key] {
				continue
			}
			seen[key] = true
			consumers = append(consumers, EmbeddedConsumer{
				ConsumerID:     anchor.Repository.ID,
				GitmodulesName: relationship.GitmodulesName,
			})
		}
	}
	sort.Slice(consumers, func(left, right int) bool {
		if consumers[left].ConsumerID != consumers[right].ConsumerID {
			return consumers[left].ConsumerID < consumers[right].ConsumerID
		}
		return consumers[left].GitmodulesName < consumers[right].GitmodulesName
	})
	return consumers
}

// EmbeddedOnlyFinding reports `GDS_WORKSPACE_EMBEDDED_ONLY` when a repository
// carries at least one incoming submodule-consumer edge and therefore may exist
// only as its superproject's gitlink. It returns nil when standalone placement
// is permitted: either no consumer edge exists (ADR 0018) or the module
// declares `development: standalone` (ADR 0042), in which case its embedded
// materializations are consumed pins beside the standalone development
// checkout.
func EmbeddedOnlyFinding(
	anchor domain.RepositoryAnchor,
	consumers []EmbeddedConsumer,
) *domain.Finding {
	if len(consumers) == 0 {
		return nil
	}
	if anchor.Module != nil && anchor.Module.Development == "standalone" {
		return nil
	}
	repositoryID := anchor.Repository.ID
	names := make([]string, 0, len(consumers))
	gitmodules := make([]string, 0, len(consumers))
	for _, consumer := range consumers {
		names = append(names, consumer.ConsumerID)
		if consumer.GitmodulesName != "" {
			gitmodules = append(gitmodules, consumer.GitmodulesName)
		}
	}
	return &domain.Finding{
		Code: "GDS_WORKSPACE_EMBEDDED_ONLY", Severity: domain.SeverityHigh,
		Message: "Repository is consumed as a Git submodule and is materialized " +
			"only as the superproject gitlink; it gets no standalone checkout.",
		Evidence: map[string]any{
			"repository_id":   repositoryID,
			"consumers":       names,
			"gitmodules_name": gitmodules,
		},
	}
}

// SuppressDeclaredDualMaterialization drops identity-index conflicts that only
// restate the declared `development: standalone` shape: a module developed in
// its own checkout legitimately claims its identity once standalone and once
// per embedded consumer gitlink (ADR 0042). A pair of two standalone checkouts
// claiming one identity is still a conflict, as is any dual claim by a module
// that did not declare standalone development.
func SuppressDeclaredDualMaterialization(
	findings []domain.Finding,
	repositories []LayoutRepository,
) []domain.Finding {
	development := map[string]string{}
	embeddedPath := map[string]bool{}
	for _, repository := range repositories {
		if repository.Anchor.Module != nil {
			development[repository.Anchor.Repository.ID] = repository.Anchor.Module.Development
		}
		if repository.SuperprojectRoot != "" {
			embeddedPath[filepath.Clean(repository.Path)] = true
		}
	}
	kept := make([]domain.Finding, 0, len(findings))
	for _, finding := range findings {
		if finding.Code == "GDS_IDENTITY_INDEX_ID_CONFLICT" {
			repositoryID, _ := finding.Evidence["repository_id"].(string)
			first, _ := finding.Evidence["first_path"].(string)
			second, _ := finding.Evidence["second_path"].(string)
			if development[repositoryID] == "standalone" &&
				(embeddedPath[filepath.Clean(first)] || embeddedPath[filepath.Clean(second)]) {
				continue
			}
		}
		kept = append(kept, finding)
	}
	return kept
}
