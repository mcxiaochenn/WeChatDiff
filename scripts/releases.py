#!/usr/bin/env python3
"""Release 即持久化状态；命令失败时终止，附件完整才公开草稿。"""
import argparse
import hashlib
import gzip
import json
import os
from pathlib import Path
import subprocess


def gh(*args):
    return subprocess.check_output(["gh", *args], text=True)


def restore(repo, root):
    root.mkdir(parents=True, exist_ok=True)
    pages = json.loads(gh("api", "--paginate", "--slurp", f"repos/{repo}/releases?per_page=100"))
    releases = [r for page in pages for r in page if not r["draft"] and r["tag_name"].startswith("apk-")]
    for release in releases:
        if not set(ASSETS).issubset(a["name"] for a in release["assets"]):
            raise RuntimeError(f"已发布状态缺少附件：{release['tag_name']}")
    (root / "known-tags.txt").write_text("\n".join(r["tag_name"] for r in releases), encoding="utf-8")
    if not releases:
        print("首次运行，没有历史基线")
        return
    latest = json.loads(gh("api", f"repos/{repo}/releases/latest"))
    if not latest["tag_name"].startswith("apk-"):
        raise RuntimeError("Latest Release 不是 WeChatDiff 状态，拒绝静默重新建立基线")
    gh("release", "download", latest["tag_name"], "--repo", repo, "--pattern", "snapshot.json.gz", "--dir", str(root))
    with gzip.open(root / "snapshot.json.gz", "rt", encoding="utf-8") as file:
        snapshot = json.load(file)
    if latest["tag_name"] != "apk-" + snapshot["identity"]["sha256"]:
        raise RuntimeError("快照身份与 Release tag 不一致")
    print("基线：", latest["tag_name"])


ASSETS = ("CHANGELOG.md", "metadata.json", "snapshot.json.gz", "diff.json.gz", "SHA256SUMS")


def verify_assets(local, remote):
    by_name = {a["name"]: a for a in remote}
    for name in ASSETS:
        asset = by_name.get(name)
        path = local / name
        if not asset or asset["size"] != path.stat().st_size:
            raise RuntimeError(f"附件不完整：{name}")
        digest = asset.get("digest")
        if digest and digest != "sha256:" + hashlib.sha256(path.read_bytes()).hexdigest():
            raise RuntimeError(f"附件校验失败：{name}")


def publish(repo, root, latest):
    metadata = json.loads((root / "metadata.json").read_text(encoding="utf-8"))
    tag = metadata["tag"]
    if tag != "apk-" + metadata["identity"]["sha256"]:
        raise RuntimeError("Tag 与 APK SHA-256 不一致")
    # 先列出草稿及已发布版本，API 错误不能被误判为“尚未发布”。
    pages = json.loads(gh("api", "--paginate", "--slurp", f"repos/{repo}/releases?per_page=100"))
    existing = next((r for page in pages for r in page if r["tag_name"] == tag), None)
    if existing and not existing["draft"]:
        print("已经发布，跳过：", existing["html_url"])
        return
    sums = []
    for name in ASSETS[:-1]:
        sums.append(f"{hashlib.sha256((root / name).read_bytes()).hexdigest()}  {name}\n")
    (root / "SHA256SUMS").write_text("".join(sums), encoding="utf-8")
    i = metadata["identity"]
    title = f"微信 Android {i['version']} · 构建 {i['build']} · {i['sha256'][:12]}"
    if not existing:
        gh("release", "create", tag, "--repo", repo, "--draft", "--target", os.environ["GITHUB_SHA"], "--title", title, "--notes-file", str(root / "CHANGELOG.md"))
    gh("release", "upload", tag, *(str(root / n) for n in ASSETS), "--repo", repo, "--clobber")
    # 按 tag 的 REST 接口仅返回已公开 Release；草稿从有写权限的列表读取。
    pages = json.loads(gh("api", "--paginate", "--slurp", f"repos/{repo}/releases?per_page=100"))
    release = next((r for page in pages for r in page if r["tag_name"] == tag), None)
    if not release:
        raise RuntimeError("上传后无法找到待发布草稿")
    verify_assets(root, release["assets"])
    gh("release", "edit", tag, "--repo", repo, "--title", title, "--notes-file", str(root / "CHANGELOG.md"), "--draft=false", f"--latest={'true' if latest else 'false'}")
    print("发布完成：", release["html_url"])


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("action", choices=("restore", "publish"))
    parser.add_argument("--repo", default=os.environ.get("GITHUB_REPOSITORY"))
    parser.add_argument("--dir", type=Path, required=True)
    parser.add_argument("--historical", action="store_true")
    args = parser.parse_args()
    if not args.repo:
        parser.error("需要 --repo 或 GITHUB_REPOSITORY")
    if args.action == "restore":
        restore(args.repo, args.dir)
    else:
        publish(args.repo, args.dir, not args.historical)
