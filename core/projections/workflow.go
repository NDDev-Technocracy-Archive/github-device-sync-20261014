package projections

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/NDDev-Technocracy-Sandbox/github-device-sync/core/domain"
)

var fullWorkflowSHA = regexp.MustCompile(`(?m)^\s*uses:\s*[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+/\.github/workflows/[A-Za-z0-9_.-]+@[0-9a-f]{40}\s*$`)

// standardGitHubHostedRunner matches the GitHub-hosted labels: the floating
// "-latest" aliases and the versioned images such as ubuntu-26.04. Matching
// only the "-latest" suffix treated a pinned hosted image as estate hardware,
// which refused it on a public repository and disabled the hosted cache.
var standardGitHubHostedRunner = regexp.MustCompile(`^(ubuntu|macos|windows)-(latest|[0-9][A-Za-z0-9.-]*)$`)

// githubHostedRunner reports whether a declared runner label is GitHub-hosted.
func githubHostedRunner(label string) bool {
	return standardGitHubHostedRunner.MatchString(label)
}

func validateGoWorkflowCaller(content []byte, anchor domain.RepositoryAnchor) error {
	if anchor.CI == nil || anchor.CI.Profile != "go" || anchor.CI.GoVersion == "" ||
		anchor.CI.BuildCommand == "" || anchor.CI.TestCommand == "" ||
		anchor.CI.WorkflowRef == "" ||
		len(anchor.Verification.Commands.PRRequired) == 0 ||
		anchor.CI.TimeoutMinutes < 1 || anchor.CI.TimeoutMinutes > 120 {
		return errors.New("Go CI repository policy is incomplete")
	}
	// A self-hosted label on a public repository is a fork-PR execution path
	// onto estate hardware, so the visibility contract decides which runners a
	// generated caller may name. GitHub-hosted labels are the only ones a public
	// repository may use, and the runner groups enforce the same rule
	// server-side; this refuses to generate the file in the first place.
	if anchor.CI.Runner != "" && !githubHostedRunner(anchor.CI.Runner) &&
		anchor.Classification.VisibilityContract != "private" {
		return fmt.Errorf(
			"runner %q is not GitHub-hosted and this repository's visibility contract is %q; "+
				"a self-hosted runner on a non-private repository lets a fork pull request "+
				"execute on estate hardware",
			anchor.CI.Runner, anchor.Classification.VisibilityContract,
		)
	}
	text := string(content)
	fullHistoryCount := strings.Count(text, "fetch_depth: 0")
	cacheDisabledCount := strings.Count(text, "cache: false")
	cacheEnabledCount := strings.Count(text, "cache: true")
	hostedRunner := githubHostedRunner(ciRunner(anchor.CI))
	for _, forbidden := range []string{
		"pull_request_target:", "secrets: inherit", "permissions: write-all",
		"@main", "@master", "@v1", "@latest",
	} {
		if strings.Contains(text, forbidden) {
			return fmt.Errorf("workflow contains forbidden construct %q", forbidden)
		}
	}
	if bytes.Count(content, []byte("uses:")) != 2 || len(fullWorkflowSHA.FindAll(content, -1)) != 2 ||
		strings.Count(text, "uses: "+anchor.CI.WorkflowRef) != 2 ||
		fullHistoryCount != 2 ||
		(hostedRunner && (cacheEnabledCount != 2 || cacheDisabledCount != 0)) ||
		(!hostedRunner && (cacheDisabledCount != 2 || cacheEnabledCount != 0)) ||
		!strings.Contains(text, "permissions: {}") ||
		!strings.Contains(text, "contents: read") ||
		!strings.Contains(text, "name: gds-ci") {
		return fmt.Errorf(
			"workflow caller does not match the closed reusable-workflow contract: uses=%d pinned=%d expected=%d full_history=%d cache_disabled=%d cache_enabled=%d",
			bytes.Count(content, []byte("uses:")), len(fullWorkflowSHA.FindAll(content, -1)),
			strings.Count(text, "uses: "+anchor.CI.WorkflowRef), fullHistoryCount,
			cacheDisabledCount, cacheEnabledCount,
		)
	}
	return nil
}
