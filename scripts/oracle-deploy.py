#!/usr/bin/env python3
"""Pull tested main commits on Oracle; keep credentials and SQLite on the VM."""
import argparse
import base64
import fcntl
import json
import os
from pathlib import Path
import re
import signal
import subprocess
import tarfile
import time
import urllib.request
import urllib.error
from datetime import datetime, timezone

ROOT = Path(__file__).resolve().parent.parent
REPOSITORY = "HidraForce/imperio-barbaro"
VOLUME = "imperio-barbaro-oracle_clanboard-data"


def run(*args, capture=True, env=None):
    return subprocess.run(args, cwd=ROOT, check=True, text=True,
                          stdout=subprocess.PIPE if capture else None, env=env).stdout


def tested_commit(data, sha):
    runs = [r for r in data["workflow_runs"] if r.get("head_sha") == sha
            and r.get("head_branch") == "main" and r.get("event") == "push"
            and r.get("path") == ".github/workflows/tests.yml"
            and r.get("repository", {}).get("full_name") == REPOSITORY]
    if not runs:
        return False
    latest = max(runs, key=lambda r: (r["id"], r.get("run_attempt", 1)))
    return latest.get("status") == "completed" and latest.get("conclusion") == "success"


def ci_passed(sha):
    url = (f"https://api.github.com/repos/{REPOSITORY}/actions/workflows/tests.yml/runs"
           f"?branch=main&event=push&head_sha={sha}&per_page=100")
    request = urllib.request.Request(url, headers={
        "Accept": "application/vnd.github+json", "User-Agent": "imperio-deploy",
        "X-GitHub-Api-Version": "2022-11-28"})
    with urllib.request.urlopen(request, timeout=20) as response:
        return tested_commit(json.load(response), sha)


def compose(release, tag, *args, capture=True):
    env = dict(os.environ, APP_IMAGE_TAG=tag)
    return run("docker", "compose", "--project-directory", str(release),
               "--env-file", str(ROOT / ".env.oracle"), "-f",
               str(release / "compose.oracle.yml"), *args, capture=capture, env=env)


def snapshot(sha):
    release = ROOT / ".deploy" / "releases" / sha
    if release.is_dir():
        return release
    release.mkdir(parents=True)
    try:
        archive = subprocess.Popen(["git", "archive", sha], cwd=ROOT, stdout=subprocess.PIPE)
        with tarfile.open(fileobj=archive.stdout, mode="r|") as source:
            source.extractall(release, filter="data")
        archive.stdout.close()
        if archive.wait() != 0:
            raise RuntimeError("git archive failed")
        return release
    except BaseException:
        import shutil
        shutil.rmtree(release)
        raise


def backup(container, image):
    folder = ROOT / "backups"
    folder.mkdir(mode=0o700, exist_ok=True)
    name = datetime.now(timezone.utc).strftime("oracle-%Y%m%dT%H%M%S%fZ.tar.gz")
    path = folder / name
    partial = path.with_suffix(path.suffix + ".partial")
    try:
        run("docker", "stop", container)
        with partial.open("xb") as output:
            subprocess.run(["docker", "run", "--rm", "--network", "none",
                            "--volumes-from", f"{container}:ro", "--entrypoint", "tar",
                            image, "-C", "/data", "-czf", "-", "."],
                           check=True, stdout=output)
        with tarfile.open(partial) as archive:
            if not archive.getnames():
                raise RuntimeError("Empty database backup")
        partial.replace(path)
    finally:
        run("docker", "start", container)
    print(f"Backup saved: {path}", flush=True)


def healthy(config):
    caddy = config["services"]["caddy"]["environment"]
    app = config["services"]["clanboard"]["environment"]
    home, dashboard = caddy["DOMAIN"], caddy["DASHBOARD_DOMAIN"]
    for host in (home, dashboard):
        if not re.fullmatch(r"[a-zA-Z0-9.-]+", host):
            raise RuntimeError("Expected DNS hostnames in Oracle configuration")
    checks = [(home, "/", None, 200), (dashboard, "/", None, 401),
              (dashboard, "/api/health", (app["APP_USERNAME"], app["APP_PASSWORD"]), 200)]
    for host, path, credentials, expected in checks:
        headers = {}
        if credentials:
            encoded = base64.b64encode(":".join(credentials).encode()).decode()
            headers["Authorization"] = f"Basic {encoded}"
        request = urllib.request.Request(f"https://{host}{path}", headers=headers)
        try:
            response = urllib.request.urlopen(request, timeout=10)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            if response.status != expected:
                raise RuntimeError(f"Unexpected HTTP status for {host}{path}")
            if credentials and json.load(response) != {"status": "ok"}:
                raise RuntimeError("Unexpected health response")


def wait_healthy(config):
    for attempt in range(12):
        try:
            healthy(config)
            return
        except Exception:
            if attempt == 11:
                raise RuntimeError("Release health checks failed") from None
            time.sleep(5)


def save_state(path, state):
    temporary = path.with_suffix(".tmp")
    temporary.write_text(json.dumps(state) + "\n")
    temporary.replace(path)


def deploy(check_only=False, retry=False):
    state_dir = ROOT / ".deploy"
    state_dir.mkdir(mode=0o700, exist_ok=True)
    with (state_dir / "operation.lock").open("w") as lock:
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            print("Another deployment or backup is running.")
            return
        if run("git", "remote", "get-url", "origin").strip() != f"https://github.com/{REPOSITORY}.git":
            raise RuntimeError("Unexpected origin; deployment refused")
        if run("git", "status", "--porcelain").strip():
            raise RuntimeError("Checkout has local changes; deployment refused")
        run("git", "fetch", "origin", "main")
        sha = run("git", "rev-parse", "FETCH_HEAD").strip()
        current = run("git", "rev-parse", "HEAD").strip()
        run("git", "merge-base", "--is-ancestor", current, sha)
        state_file = state_dir / "state.json"
        state = json.loads(state_file.read_text()) if state_file.exists() else {}
        if state.get("sha") == sha and not check_only:
            return
        if state.get("failed_sha") == sha and not retry and not check_only:
            print(f"Release {sha} previously failed; inspect logs and use --retry.")
            return
        if not ci_passed(sha):
            print(f"Waiting for successful main push tests on {sha}.")
            return
        old = json.loads(run("docker", "inspect", "imperio-barbaro-oracle-clanboard-1"))[0]
        mounts = [m for m in old["Mounts"] if m["Destination"] == "/data"]
        if len(mounts) != 1 or mounts[0].get("Name") != VOLUME or mounts[0]["Type"] != "volume":
            raise RuntimeError("Unexpected database volume; deployment refused")
        if not old["State"]["Running"]:
            raise RuntimeError("Existing app must be running before deployment")
        if check_only:
            print(f"Ready: tested main {sha}, existing data volume {VOLUME}.")
            return
        release = snapshot(sha)
        tag = f"git-{sha}"
        config = json.loads(compose(release, tag, "config", "--format", "json"))
        if config.get("name") != "imperio-barbaro-oracle" or config["volumes"]["clanboard-data"]["name"] != VOLUME:
            raise RuntimeError("Release changed the production project or data volume")
        compose(release, tag, "build", "clanboard", capture=False)
        previous = snapshot(current)
        previous_tag = f"rollback-{current}"
        run("docker", "tag", old["Image"], f"imperio-barbaro:{previous_tag}")
        backup(old["Id"], old["Image"])
        try:
            compose(release, tag, "up", "-d", "--no-build", "--force-recreate", capture=False)
            wait_healthy(config)
        except Exception:
            state["failed_sha"] = sha
            save_state(state_file, state)
            compose(previous, previous_tag, "up", "-d", "--no-build", "--force-recreate", capture=False)
            wait_healthy(json.loads(compose(previous, previous_tag, "config", "--format", "json")))
            print("Previous code restored and health checked. Database preserved; inspect migration compatibility.", flush=True)
            raise
        run("git", "merge", "--ff-only", sha)
        save_state(state_file, {"sha": sha, "image_tag": tag})
        print(f"Deployed and verified {sha}.", flush=True)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true", help="Check readiness without building or deploying")
    parser.add_argument("--retry", action="store_true", help="Retry a failed release after investigation")
    args = parser.parse_args()
    os.umask(0o077)
    def interrupted(signum, frame):
        raise RuntimeError("Deployment interrupted")
    signal.signal(signal.SIGTERM, interrupted)
    try:
        deploy(args.check, args.retry)
    except Exception as error:
        print(f"Deployment stopped: {type(error).__name__}: {error}", flush=True)
        raise SystemExit(1)
