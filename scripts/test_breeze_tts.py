#!/usr/bin/env python3
"""Offline smoke tests: no network, packages, model weights or GPU are used."""

import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "scripts" / "breeze-tts.sh"
COMMIT = "daa6cb2efe5471f09369229f322d20bb956cb3c3"
STUB = r'''
import json
import os
from pathlib import Path
import shutil
import sys

kind = "git" if Path(sys.argv[0]).name == "git" else "python"
args = sys.argv[1:]
with open(os.environ["BREEZE_TEST_LOG"], "a") as log:
    log.write(json.dumps({"kind": kind, "args": args, "cwd": os.getcwd()}) + "\n")
if kind == "git":
    cwd = Path.cwd()
    if args[:1] == ["-C"]:
        cwd, args = Path(args[1]), args[2:]
    if args[:1] == ["init"]:
        (Path(args[-1]) / ".git").mkdir(parents=True)
    elif args[:2] == ["remote", "get-url"]:
        print(os.environ.get("BREEZE_TEST_ORIGIN", "https://github.com/breezeblue-ai/breeze-tts.git"))
    elif args[:1] == ["rev-parse"]:
        print(os.environ.get("BREEZE_TEST_COMMIT", "daa6cb2efe5471f09369229f322d20bb956cb3c3"))
    elif args[:1] == ["status"]:
        print(os.environ.get("BREEZE_TEST_DIRTY", ""), end="")
    elif args[:1] == ["checkout"]:
        (cwd / "requirements.txt").write_text("# Stub only: never install actual dependencies.\n")
elif args[:2] == ["-m", "venv"]:
    target = Path(args[2])
    (target / "bin").mkdir(parents=True)
    (target / "pyvenv.cfg").write_text("stub = true\n")
    shutil.copyfile(sys.argv[0], target / "bin" / "python")
    (target / "bin" / "python").chmod(0o700)
    if os.environ.get("BREEZE_TEST_FAIL_VENV_PIP"):
        (target / ".pip-missing").touch()
        sys.exit(1)
elif args[:2] == ["-m", "pip"] and (Path(sys.argv[0]).parents[1] / ".pip-missing").exists():
    sys.exit(1)
elif args[:2] == ["-m", "ensurepip"]:
    if os.environ.get("BREEZE_TEST_FAIL_ENSUREPIP"):
        sys.exit(1)
    (Path(sys.argv[0]).parents[1] / ".pip-missing").unlink(missing_ok=True)
elif args[:3] == ["-m", "pip", "install"] and os.environ.get("BREEZE_TEST_FAIL_PIP"):
    sys.exit(1)
elif args[:1] == ["-c"] and "os.path.realpath" in args[1]:
    print(os.path.realpath(args[2]))
'''


class BreezeScriptSmoke(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="ainovel-breeze-smoke-")
        self.addCleanup(self.temp.cleanup)
        self.directory = Path(self.temp.name)
        self.bin = self.directory / "bin"
        self.bin.mkdir()
        for name in ("git", "mock-python"):
            path = self.bin / name
            path.write_text(f"#!{sys.executable}\n" + STUB)
            path.chmod(0o700)
        self.runtime = self.directory / "runtime with spaces"
        self.log = self.directory / "calls.jsonl"
        self.env = os.environ.copy()
        for key in list(self.env):
            if key.startswith("BREEZE_"):
                del self.env[key]
        self.env.update(
            PATH=str(self.bin) + os.pathsep + os.environ["PATH"],
            BREEZE_TTS_HOME=str(self.runtime),
            BREEZE_TTS_PYTHON=str(self.bin / "mock-python"),
            BREEZE_TEST_LOG=str(self.log),
        )

    def run_script(self, command, *, ok=True, **variables):
        result = subprocess.run(
            ["bash", str(SCRIPT), command],
            cwd=self.directory,
            env={**self.env, **variables},
            text=True,
            capture_output=True,
            timeout=10,
        )
        if ok:
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        else:
            self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        return result

    def calls(self):
        if not self.log.exists():
            return []
        return [json.loads(line) for line in self.log.read_text().splitlines()]

    def test_help_does_not_install(self):
        self.assertIn("research/non-commercial", self.run_script("help").stdout)
        self.assertFalse(self.runtime.exists())
        self.assertEqual(self.calls(), [])
        self.run_script("invalid", ok=False)

    def test_install_is_pinned_and_idempotent(self):
        self.run_script("install")
        self.run_script("install")
        self.assertEqual((self.runtime / ".installed-commit").read_text().strip(), COMMIT)
        self.assertFalse((self.runtime / ".install-lock").exists())
        calls = self.calls()
        self.assertEqual(sum(c["args"][:3] == ["-m", "pip", "install"] for c in calls), 1)
        fetches = [c for c in calls if "fetch" in c["args"]]
        self.assertEqual(len(fetches), 1)
        self.assertEqual(fetches[0]["args"][-1], COMMIT)
        self.assertFalse(any(c["args"][:2] == ["-m", "breeze_infer.api"] for c in calls))

    def test_failed_install_can_resume(self):
        self.run_script("install", ok=False, BREEZE_TEST_FAIL_PIP="1")
        self.assertFalse((self.runtime / ".installed-commit").exists())
        self.assertFalse((self.runtime / ".install-lock").exists())
        self.run_script("install")
        self.assertEqual(sum("fetch" in c["args"] for c in self.calls()), 1)

    def test_failed_venv_bootstrap_can_resume_without_recreating_environment(self):
        self.run_script("install", ok=False, BREEZE_TEST_FAIL_VENV_PIP="1")
        self.assertFalse((self.runtime / ".installed-commit").exists())
        venv = self.runtime / "venv"
        self.assertTrue((venv / "pyvenv.cfg").is_file())
        self.assertTrue((venv / "bin" / "python").is_file())
        marker = venv / "keep-existing-files"
        marker.write_text("user file")
        failed = self.run_script("install", ok=False, BREEZE_TEST_FAIL_ENSUREPIP="1")
        self.assertIn("venv/ensurepip system package", failed.stderr)
        self.assertFalse((self.runtime / ".installed-commit").exists())
        self.assertFalse((self.runtime / ".install-lock").exists())
        self.run_script("install")
        self.assertEqual((self.runtime / ".installed-commit").read_text().strip(), COMMIT)
        self.assertEqual(marker.read_text(), "user file")
        calls = self.calls()
        self.assertEqual(sum(c["args"][:2] == ["-m", "venv"] for c in calls), 1)
        self.assertEqual(sum(c["args"][:3] == ["-m", "ensurepip", "--upgrade"] for c in calls), 2)
        self.assertEqual(sum("fetch" in c["args"] for c in calls), 1)

    def test_existing_source_is_not_overwritten(self):
        self.run_script("install")
        for variables in (
            {"BREEZE_TEST_COMMIT": "another-commit"},
            {"BREEZE_TEST_ORIGIN": "https://example.test/another-repo.git"},
            {"BREEZE_TEST_DIRTY": " M user-change.py"},
        ):
            for command in ("install", "start"):
                with self.subTest(command=command, variables=variables):
                    self.run_script(command, ok=False, **variables)
        self.assertEqual(sum("checkout" in c["args"] for c in self.calls()), 1)

    def test_start_requires_install_and_checks_arguments(self):
        self.run_script("start", ok=False)
        self.run_script("install")
        for port in ("0", "65536", "-1", "invalid", "$(touch should-not-exist)"):
            with self.subTest(port=port):
                self.run_script("start", ok=False, BREEZE_TTS_PORT=port)
        self.run_script("start", ok=False, BREEZE_TTS_FAST="yes")
        self.assertFalse((self.directory / "should-not-exist").exists())
        self.assertFalse(any(c["args"][:2] == ["-m", "breeze_infer.api"] for c in self.calls()))

    def test_start_preserves_model_path_and_environment(self):
        self.run_script("install")
        checkpoint = self.directory / "checkpoint with spaces"
        checkpoint.mkdir()
        self.run_script(
            "start",
            BREEZE_TTS_MODEL=checkpoint.name,
            BREEZE_TTS_HOST="0.0.0.0",
            BREEZE_TTS_PORT="8765",
            BREEZE_TTS_FAST="1",
            BREEZE_TTS_REVISION="model-revision",
        )
        request = next(c for c in self.calls() if c["args"][:2] == ["-m", "breeze_infer.api"])
        self.assertEqual(request["cwd"], str(self.runtime / "source"))
        self.assertEqual(request["args"], [
            "-m", "breeze_infer.api", str(checkpoint), "--host", "0.0.0.0",
            "--port", "8765", "--revision", "model-revision", "--fast-all",
        ])

    def test_start_defaults_to_loopback_and_eager(self):
        self.run_script("install")
        self.run_script("start")
        request = next(c for c in self.calls() if c["args"][:2] == ["-m", "breeze_infer.api"])
        self.assertEqual(request["args"], [
            "-m", "breeze_infer.api", "BreezeBlue/breeze-tts-2",
            "--host", "127.0.0.1", "--port", "7860",
        ])

    def test_check_is_offline_and_does_not_generate(self):
        self.run_script("install")
        self.log.write_text("")
        self.run_script("check")
        self.assertFalse(any("fetch" in c["args"] or "install" in c["args"] for c in self.calls()))
        self.assertFalse(any(c["args"][:2] == ["-m", "breeze_infer.api"] for c in self.calls()))

    def test_refuses_checkout_as_runtime_directory(self):
        result = self.run_script("install", ok=False, BREEZE_TTS_HOME=str(ROOT))
        self.assertIn("outside the application checkout", result.stderr)

    def test_refuses_runtime_paths_resolving_to_root(self):
        result = self.run_script("install", ok=False, BREEZE_TTS_HOME="/tmp/..")
        self.assertIn("must not resolve to /", result.stderr)
        self.assertFalse(any("fetch" in c["args"] for c in self.calls()))

    def test_install_resolves_symlinks_before_creating_files(self):
        alias = self.directory / "checkout-alias"
        alias.symlink_to(ROOT, target_is_directory=True)
        result = self.run_script("install", ok=False, BREEZE_TTS_HOME=str(alias / "not-created-breeze-test"))
        self.assertIn("outside the application checkout", result.stderr)
        self.assertFalse((ROOT / "not-created-breeze-test").exists())


if __name__ == "__main__":
    unittest.main()
