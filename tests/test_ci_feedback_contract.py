"""Small wiring checks; publisher behavior belongs to the STD-0004 library."""
import pathlib
import json
import re
import yaml
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]
CALLER = ROOT / ".github/workflows/ci-feedback-events.yml"
LIBRARY_ACTION = "NDDev-Technocracy-Standarts/std-0004-ci-cd-workflows/.github/actions/feedback"


def _watched():
    """Return (workflow names, workflow paths) that the publisher must watch."""
    names, paths = [], []
    for path in sorted((ROOT / ".github/workflows").glob("*.yml")):
        if path == CALLER:
            continue
        document = yaml.safe_load(path.read_text())
        events = document.get("on", document.get(True))
        if isinstance(events, str):
            events = [events]
        if set(events) - {"workflow_call"}:
            names.append(document.get("name", path.name))
            paths.append(f".github/workflows/{path.name}")
    return names, paths


def _step():
    steps = yaml.safe_load(CALLER.read_text())["jobs"]["feedback"]["steps"]
    return next(step for step in steps if step.get("uses", "").startswith(LIBRARY_ACTION + "@"))


class FeedbackContractTests(unittest.TestCase):
    def test_exact_completed_run_is_forwarded(self):
        text = CALLER.read_text()
        selected = json.loads(re.search(r"workflows: (\[[^\n]+\])", text).group(1))
        names, paths = _watched()
        self.assertEqual(selected, names)
        self.assertNotIn("CI feedback", selected)
        self.assertIn("types: [completed]", text)
        step = _step()
        self.assertEqual(json.loads(step["with"]["workflows"]), paths)
        self.assertIn("github.event.workflow_run.id", step["with"]["run-id"])
        self.assertNotIn("github.run_id", json.dumps(step["with"]))

    def test_library_reference_is_immutable(self):
        text = CALLER.read_text()
        self.assertRegex(
            text,
            r"uses: NDDev-Technocracy-Standarts/std-0004-ci-cd-workflows/\.github/actions/feedback@[0-9a-f]{40} # nddev-cicd-v\d+\.\d+\.\d+\n",
        )
        self.assertNotIn("ci-workflows-20261014", text)
        self.assertNotIn("@main", text)

    def test_no_project_execution_or_secret_inheritance(self):
        text = CALLER.read_text()
        document = yaml.safe_load(text)
        job = document["jobs"]["feedback"]
        self.assertEqual(document["permissions"], {})
        self.assertEqual(job["permissions"], {"actions": "read", "issues": "write"})
        self.assertEqual([step.get("uses", "").split("@")[0] for step in job["steps"]], [LIBRARY_ACTION])
        self.assertNotIn("checkout", text)
        self.assertNotIn("secrets", text)
        self.assertNotIn("run:", text.replace("workflow_run:", ""))
        self.assertEqual(_step()["env"], {"GITHUB_TOKEN": "${{ github.token }}"})

    def test_failed_conclusions_reach_publisher(self):
        text = CALLER.read_text()
        condition = next(line for line in text.splitlines() if line.strip().startswith("if:"))
        for conclusion in ("failure", "timed_out", "startup_failure"):
            self.assertIn(f'"{conclusion}"', condition)
        self.assertNotIn("continue-on-error", text)


if __name__ == "__main__":
    unittest.main()
