"""验证发布事务：API 失败不创建，附件缺失不公开，已有 Release 不修改。"""
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import releases


class ReleaseTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.root = Path(self.temp.name)
        self.addCleanup(self.temp.cleanup)
        for name in releases.ASSETS[:-1]:
            (self.root / name).write_text("data", encoding="utf-8")
        metadata = {"tag": "apk-" + "a" * 64, "identity": {"sha256": "a" * 64, "version": "8.0.79", "build": "3200"}}
        (self.root / "metadata.json").write_text(json.dumps(metadata), encoding="utf-8")

    def test_api_failure_does_not_create(self):
        with patch.object(releases, "gh", side_effect=RuntimeError("HTTP 403")) as gh:
            with self.assertRaises(RuntimeError):
                releases.publish("owner/repo", self.root, True)
            self.assertEqual(gh.call_count, 1)

    def test_published_is_immutable(self):
        existing = [[{"tag_name": "apk-" + "a" * 64, "draft": False, "html_url": "url"}]]
        with patch.object(releases, "gh", return_value=json.dumps(existing)) as gh:
            releases.publish("owner/repo", self.root, True)
            self.assertEqual(gh.call_count, 1)

    def test_missing_asset_keeps_draft(self):
        calls = []
        def gh(*args, **kwargs):
            calls.append(args)
            if args[0] == "api" and "--slurp" in args:
                if len(calls)==1:
                    return "[[]]"
                return json.dumps([[{"tag_name":"apk-"+"a"*64,"assets":[],"html_url":"url"}]])
            return ""
        with patch.object(releases, "gh", side_effect=gh), patch.dict(os.environ, {"GITHUB_SHA": "commit"}):
            with self.assertRaises(RuntimeError):
                releases.publish("owner/repo", self.root, True)
        self.assertFalse(any("PATCH" in c for c in calls))

    def test_resume_draft_and_publish(self):
        calls = []
        bodies = []
        def gh(*args, **kwargs):
            calls.append(args)
            if "body" in kwargs:
                bodies.append(kwargs["body"])
            if args[0] == "api" and "--slurp" in args:
                if len(calls)==1:
                    return json.dumps([[{"tag_name": "apk-" + "a" * 64, "draft": True}]])
                return json.dumps([[{"id":1,"tag_name":"apk-"+"a"*64,"html_url": "url", "assets": [{"name": n, "size": (self.root / n).stat().st_size} for n in releases.ASSETS]}]])
            return ""
        with patch.object(releases, "gh", side_effect=gh):
            releases.publish("owner/repo", self.root, False)
        self.assertFalse(any(c[:2] == ("release", "create") for c in calls))
        self.assertIn("PATCH", calls[-1])
        self.assertEqual(bodies[-1]["make_latest"], "false")
        self.assertFalse(bodies[-1]["draft"])

    def test_published_wins_over_duplicate_draft(self):
        existing = [[{"tag_name":"apk-"+"a"*64,"draft":True},
                     {"tag_name":"apk-"+"a"*64,"draft":False,"html_url":"url"}]]
        with patch.object(releases,"gh",return_value=json.dumps(existing)) as gh:
            releases.publish("owner/repo",self.root,True)
            self.assertEqual(gh.call_count,1)

    def test_select_complete_duplicate_draft(self):
        calls = []
        tag = "apk-" + "a" * 64
        def gh(*args, **kwargs):
            calls.append(args)
            if "--slurp" in args:
                if len(calls)==1:
                    return json.dumps([[{"tag_name":tag,"draft":True}]])
                return json.dumps([[{"id":2,"tag_name":tag,"assets":[]},
                    {"id":1,"tag_name":tag,"html_url":"url","assets":[{"name":n,"size":(self.root/n).stat().st_size} for n in releases.ASSETS]}]])
            return ""
        with patch.object(releases,"gh",side_effect=gh):
            releases.publish("owner/repo",self.root,True)
        self.assertIn("repos/owner/repo/releases/1",calls[-1])

    def test_digest_mismatch(self):
        for n in releases.ASSETS:
            if not (self.root / n).exists():
                (self.root / n).write_text("sum")
        remote = [{"name": n, "size": (self.root / n).stat().st_size, "digest": "sha256:wrong"} for n in releases.ASSETS]
        with self.assertRaises(RuntimeError):
            releases.verify_assets(self.root, remote)


if __name__ == "__main__":
    unittest.main()
