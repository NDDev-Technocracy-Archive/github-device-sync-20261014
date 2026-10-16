package git

import (
	"bytes"
	"context"
	"fmt"
)

// PathIgnored reports whether relative is excluded inside directory by the
// repository's own ignore machinery (.gitignore, .git/info/exclude and the
// configured core.excludesFile). The index is consulted, so a path that is
// tracked -- a committed gitlink for an embedded checkout is the case that
// matters -- is never reported ignored, while generated materialization such
// as a vendored tool cache inside an ignored build tree is.
func (runner *Runner) PathIgnored(
	ctx context.Context, directory string, relative string,
) (bool, error) {
	result, err := runner.run(
		ctx, directory, map[int]struct{}{0: {}},
		"status", "--porcelain=v2", "-z", "--untracked-files=all", "--ignored",
		"--", relative,
	)
	if err != nil {
		return false, fmt.Errorf("git status --ignored %q in %s: %w", relative, directory, err)
	}
	for _, record := range bytes.Split(result.Stdout, []byte{0}) {
		if len(record) > 2 && record[0] == '!' {
			return true, nil
		}
	}
	return false, nil
}
