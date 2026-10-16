import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("deploy", Path(__file__).with_name("oracle-deploy.py"))
deploy = importlib.util.module_from_spec(spec)
spec.loader.exec_module(deploy)
SHA = "a" * 40


def workflow():
    return dict(id=10, run_attempt=1, head_sha=SHA, head_branch="main", event="push",
                path=".github/workflows/tests.yml", repository={"full_name": deploy.REPOSITORY},
                status="completed", conclusion="success")


class DeploymentTests(unittest.TestCase):
    def exercise_release(self, failed_health=False):
        current = "b" * 40
        config = {"name": "imperio-barbaro-oracle", "volumes": {"clanboard-data": {"name": deploy.VOLUME}}}
        container = {"Id": "app-container", "Image": "old-image", "State": {"Running": True},
                     "Mounts": [{"Destination": "/data", "Type": "volume", "Name": deploy.VOLUME}]}
        with tempfile.TemporaryDirectory() as folder, patch.object(deploy, "ROOT", Path(folder)), \
             patch.object(deploy, "ci_passed", return_value=True), patch.object(deploy, "run") as run, \
             patch.object(deploy, "compose") as compose, patch.object(deploy, "snapshot") as snapshot, \
             patch.object(deploy, "backup") as backup, patch.object(deploy, "wait_healthy") as health:
            run.side_effect = lambda *a, **k: (f"https://github.com/{deploy.REPOSITORY}.git\n" if a[:3] == ("git", "remote", "get-url")
                else SHA if a == ("git", "rev-parse", "FETCH_HEAD") else current if a == ("git", "rev-parse", "HEAD")
                else json.dumps([container]) if a[:2] == ("docker", "inspect") else "")
            compose.return_value = json.dumps(config)
            snapshot.side_effect = [Path(folder) / SHA, Path(folder) / current]
            if failed_health:
                health.side_effect = [RuntimeError("unhealthy"), None]
                with self.assertRaisesRegex(RuntimeError, "unhealthy"):
                    deploy.deploy()
            else:
                deploy.deploy()
            state = json.loads((Path(folder) / ".deploy/state.json").read_text())
            backup.assert_called_once_with("app-container", "old-image")
            ups = [c.args for c in compose.call_args_list if "up" in c.args]
            if failed_health:
                self.assertEqual(state["failed_sha"], SHA)
                self.assertNotIn("sha", state)
                self.assertEqual(ups[-1][1], f"rollback-{current}")
                self.assertFalse(any(c.args[:2] == ("git", "merge") for c in run.call_args_list))
            else:
                self.assertEqual(state["sha"], SHA)
                self.assertEqual(len(ups), 1)
                self.assertTrue(any(c.args == ("git", "merge", "--ff-only", SHA) for c in run.call_args_list))
            compose.reset_mock()
            deploy.deploy()
            compose.assert_not_called()

    def test_successful_release_is_recorded_and_not_redeployed(self):
        self.exercise_release()

    def test_failed_health_restores_previous_image_and_blocks_repeat(self):
        self.exercise_release(failed_health=True)

    def test_accepts_only_successful_production_test_run(self):
        self.assertTrue(deploy.tested_commit({"workflow_runs": [workflow()]}, SHA))
        for field, value in [("head_sha", "b" * 40), ("head_branch", "dev"),
                             ("event", "pull_request"), ("path", ".github/workflows/other.yml"),
                             ("repository", {"full_name": "someone/fork"}),
                             ("status", "in_progress"), ("conclusion", "failure")]:
            run = workflow()
            run[field] = value
            with self.subTest(field=field):
                self.assertFalse(deploy.tested_commit({"workflow_runs": [run]}, SHA))

    def test_newer_pending_or_failed_run_blocks_older_success(self):
        for status, conclusion in [("in_progress", None), ("completed", "failure")]:
            newer = workflow()
            newer.update(id=11, status=status, conclusion=conclusion)
            self.assertFalse(deploy.tested_commit({"workflow_runs": [newer, workflow()]}, SHA))
        self.assertFalse(deploy.tested_commit({"workflow_runs": []}, SHA))

    def test_backup_restarts_app_after_archive_failure(self):
        with tempfile.TemporaryDirectory() as folder, patch.object(deploy, "ROOT", Path(folder)), \
             patch.object(deploy, "run") as run, \
             patch.object(deploy.subprocess, "run", side_effect=RuntimeError("archive failed")):
            with self.assertRaises(RuntimeError):
                deploy.backup("container", "image")
            self.assertEqual(run.call_args_list[-1].args, ("docker", "start", "container"))

    def test_failed_ci_never_touches_docker(self):
        with tempfile.TemporaryDirectory() as folder, patch.object(deploy, "ROOT", Path(folder)), \
             patch.object(deploy, "ci_passed", return_value=False), \
             patch.object(deploy, "run") as run:
            run.side_effect = [f"https://github.com/{deploy.REPOSITORY}.git\n", "", "", SHA, SHA, ""]
            deploy.deploy()
            self.assertFalse(any(call.args[0] == "docker" for call in run.call_args_list))

    def test_unexpected_volume_blocks_build_backup_and_deploy(self):
        container = {"Mounts": [{"Destination": "/data", "Name": "wrong", "Type": "volume"}]}
        with tempfile.TemporaryDirectory() as folder, patch.object(deploy, "ROOT", Path(folder)), \
             patch.object(deploy, "ci_passed", return_value=True), \
             patch.object(deploy, "run") as run, patch.object(deploy, "compose") as compose, \
             patch.object(deploy, "backup") as backup:
            run.side_effect = [f"https://github.com/{deploy.REPOSITORY}.git\n", "", "", SHA, SHA, "", json.dumps([container])]
            with self.assertRaisesRegex(RuntimeError, "database volume"):
                deploy.deploy()
            compose.assert_not_called()
            backup.assert_not_called()

if __name__ == "__main__":
    unittest.main()
