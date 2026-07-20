#!/usr/bin/env python3
"""
mac-inventory: 맥에 설치된 소프트웨어 전체를 스캔해서 JSON 인벤토리로 만드는 프로토타입.

스캔 대상:
  - GUI 앱 (/Applications, ~/Applications)  : Bundle ID, 버전, 설치일, 용량
  - Homebrew (formulae + casks)             : 직접 설치 여부(leaves), 최신버전 여부
  - npm 글로벌 패키지                        : 최신버전 여부
  - pipx, cargo, mas(App Store), ~/go/bin   : 설치되어 있으면 자동 감지
  - LaunchAgents / LaunchDaemons            : 백그라운드 잔존물 후보

사용법:
  python3 mac_inventory.py                  # 전체 스캔 + 최신버전 체크
  python3 mac_inventory.py --no-outdated    # 네트워크 조회(느림) 생략
  python3 mac_inventory.py --no-sizes       # 앱 용량 계산(du) 생략
  python3 mac_inventory.py -o report.json   # 출력 파일 지정

의존성: 없음 (Python 3.9+ 표준 라이브러리만 사용)
"""

from __future__ import annotations

import argparse
import concurrent.futures
import json
import os
import plistlib
import re
import shutil
import subprocess
import sys
from datetime import datetime, timezone
from pathlib import Path

HOME = Path.home()
ERRORS: list[str] = []


# ---------------------------------------------------------------- 공통 유틸

def run(cmd: list[str], timeout: int = 120, ok_codes: tuple = (0,)) -> str | None:
    """명령 실행. 실패하면 None을 돌려주고 ERRORS에 기록한다."""
    try:
        p = subprocess.run(
            cmd, capture_output=True, text=True, timeout=timeout,
        )
        if p.returncode not in ok_codes and not p.stdout.strip():
            ERRORS.append(f"{' '.join(cmd)}: exit {p.returncode} {p.stderr.strip()[:200]}")
            return None
        return p.stdout
    except FileNotFoundError:
        return None
    except subprocess.TimeoutExpired:
        ERRORS.append(f"{' '.join(cmd)}: timeout")
        return None
    except Exception as e:  # noqa: BLE001
        ERRORS.append(f"{' '.join(cmd)}: {e}")
        return None


def run_json(cmd: list[str], timeout: int = 120, ok_codes: tuple = (0,)):
    out = run(cmd, timeout=timeout, ok_codes=ok_codes)
    if not out or not out.strip():
        return None
    try:
        return json.loads(out)
    except json.JSONDecodeError:
        ERRORS.append(f"{' '.join(cmd)}: JSON 파싱 실패")
        return None


def human_size(kb: int | None) -> str:
    if kb is None:
        return "-"
    if kb >= 1024 * 1024:
        return f"{kb / 1024 / 1024:.1f} GB"
    if kb >= 1024:
        return f"{kb / 1024:.0f} MB"
    return f"{kb} KB"


# ---------------------------------------------------------------- 1. GUI 앱

def _mdls_values(app: Path, names: list[str]) -> list[str | None]:
    """mdls -raw는 여러 속성을 \\0 로 구분해 출력한다."""
    cmd = ["mdls", "-raw"]
    for n in names:
        cmd += ["-name", n]
    cmd.append(str(app))
    out = run(cmd, timeout=20)
    if out is None:
        return [None] * len(names)
    vals = out.split("\0")
    vals += [None] * (len(names) - len(vals))
    return [None if (v is None or v.strip() in ("(null)", "")) else v.strip() for v in vals]


def _du_kb(path: Path) -> int | None:
    out = run(["du", "-sk", str(path)], timeout=60)
    if out:
        try:
            return int(out.split()[0])
        except (ValueError, IndexError):
            pass
    return None


def _scan_one_app(app: Path, with_size: bool) -> dict:
    bundle_id, version, date_added = _mdls_values(
        app, ["kMDItemCFBundleIdentifier", "kMDItemVersion", "kMDItemDateAdded"]
    )
    if date_added is None:
        try:
            st = app.stat()
            birth = getattr(st, "st_birthtime", st.st_mtime)
            date_added = datetime.fromtimestamp(birth).strftime("%Y-%m-%d %H:%M:%S %z").strip()
        except OSError:
            pass
    return {
        "name": app.stem,
        "path": str(app),
        "bundle_id": bundle_id,
        "version": version,
        "date_added": date_added,
        "size_kb": _du_kb(app) if with_size else None,
    }


def scan_apps(with_size: bool) -> list[dict]:
    roots = [Path("/Applications"), HOME / "Applications", Path("/Applications/Utilities")]
    apps: list[Path] = []
    for root in roots:
        if not root.is_dir():
            continue
        try:
            for entry in sorted(root.iterdir()):
                if entry.suffix == ".app":
                    apps.append(entry)
        except PermissionError:
            ERRORS.append(f"권한 없음: {root}")

    results: list[dict] = []
    with concurrent.futures.ThreadPoolExecutor(max_workers=8) as ex:
        futures = [ex.submit(_scan_one_app, a, with_size) for a in apps]
        for f in concurrent.futures.as_completed(futures):
            results.append(f.result())
    results.sort(key=lambda x: x["name"].lower())
    return results


# ---------------------------------------------------------------- 2. Homebrew

def scan_brew() -> dict | None:
    if not shutil.which("brew"):
        return None
    prefix = (run(["brew", "--prefix"], timeout=30) or "").strip() or None
    info = run_json(["brew", "info", "--json=v2", "--installed"], timeout=300)
    if info is None:
        return {"prefix": prefix, "formulae": [], "casks": []}

    formulae = []
    for f in info.get("formulae", []):
        inst = (f.get("installed") or [{}])[0]
        formulae.append({
            "name": f.get("name"),
            "version": inst.get("version"),
            "installed_on_request": bool(inst.get("installed_on_request")),  # 직접 설치 (leaves)
            "installed_as_dependency": bool(inst.get("installed_as_dependency")),
            "dependencies": f.get("dependencies", []),
            "latest_version": None,   # outdated 단계에서 채움
            "outdated": False,
        })

    casks = []
    for c in info.get("casks", []):
        casks.append({
            "name": c.get("token"),
            "version": c.get("installed"),
            "app_names": [a for a in (c.get("artifacts") or []) if isinstance(a, dict) is False and str(a).endswith(".app")],
            "latest_version": None,
            "outdated": False,
        })

    return {"prefix": prefix, "formulae": formulae, "casks": casks}


def check_brew_outdated(brew: dict) -> None:
    data = run_json(["brew", "outdated", "--json=v2"], timeout=300)
    if data is None:
        return
    by_name_f = {f["name"]: f for f in brew["formulae"]}
    for o in data.get("formulae", []):
        f = by_name_f.get(o.get("name"))
        if f:
            f["outdated"] = True
            f["latest_version"] = o.get("current_version")
    by_name_c = {c["name"]: c for c in brew["casks"]}
    for o in data.get("casks", []):
        c = by_name_c.get(o.get("name"))
        if c:
            c["outdated"] = True
            c["latest_version"] = o.get("current_version")


# ---------------------------------------------------------------- 3. npm 글로벌

def scan_npm() -> dict | None:
    if not shutil.which("npm"):
        return None
    root = (run(["npm", "root", "-g"], timeout=60) or "").strip() or None
    # npm ls는 문제가 있어도 exit 1을 낼 수 있으므로 허용
    data = run_json(["npm", "ls", "-g", "--depth=0", "--json"], timeout=120, ok_codes=(0, 1))
    packages = []
    for name, meta in ((data or {}).get("dependencies") or {}).items():
        packages.append({
            "name": name,
            "version": meta.get("version"),
            "latest_version": None,
            "outdated": False,
        })
    packages.sort(key=lambda x: x["name"])
    return {"root": root, "packages": packages}


def check_npm_outdated(npm: dict) -> None:
    # outdated 패키지가 있으면 exit code 1 이 정상 동작
    data = run_json(["npm", "outdated", "-g", "--json"], timeout=180, ok_codes=(0, 1))
    if not data:
        return
    by_name = {p["name"]: p for p in npm["packages"]}
    for name, meta in data.items():
        p = by_name.get(name)
        if p:
            p["outdated"] = True
            p["latest_version"] = meta.get("latest")


# ---------------------------------------------------------------- 4. 기타 소스

def scan_pipx() -> list[dict] | None:
    if not shutil.which("pipx"):
        return None
    data = run_json(["pipx", "list", "--json"], timeout=60)
    if not data:
        return []
    out = []
    for name, venv in (data.get("venvs") or {}).items():
        pkg = ((venv.get("metadata") or {}).get("main_package") or {})
        out.append({"name": name, "version": pkg.get("package_version")})
    return sorted(out, key=lambda x: x["name"])


def scan_cargo() -> list[dict] | None:
    if not shutil.which("cargo"):
        return None
    out = run(["cargo", "install", "--list"], timeout=60)
    if out is None:
        return []
    pkgs = []
    for line in out.splitlines():
        if line and not line.startswith((" ", "\t")):
            m = re.match(r"^(\S+)\s+v?([\w.\-+]+)", line.rstrip(":"))
            if m:
                pkgs.append({"name": m.group(1), "version": m.group(2)})
    return pkgs


def scan_mas() -> dict | None:
    """App Store 설치 앱 (mas CLI가 있을 때만)."""
    if not shutil.which("mas"):
        return None
    apps, outdated = [], []
    out = run(["mas", "list"], timeout=60)
    for line in (out or "").splitlines():
        m = re.match(r"^(\d+)\s+(.+?)\s+\(([^)]*)\)\s*$", line.strip())
        if m:
            apps.append({"id": m.group(1), "name": m.group(2), "version": m.group(3)})
    out = run(["mas", "outdated"], timeout=120)
    for line in (out or "").splitlines():
        m = re.match(r"^(\d+)\s+(.+?)\s+\(([^)]*)\s*->\s*([^)]*)\)\s*$", line.strip())
        if m:
            outdated.append({"id": m.group(1), "name": m.group(2),
                             "version": m.group(3).strip(), "latest_version": m.group(4).strip()})
    return {"apps": apps, "outdated": outdated}


def scan_go_bin() -> list[dict] | None:
    gobin = Path(os.environ.get("GOBIN") or (HOME / "go" / "bin"))
    if not gobin.is_dir():
        return None
    out = []
    for f in sorted(gobin.iterdir()):
        if f.is_file() and os.access(f, os.X_OK):
            out.append({"name": f.name, "path": str(f)})
    return out


# ---------------------------------------------------------------- 5. launchd

def scan_launchd() -> list[dict]:
    dirs = [
        (HOME / "Library" / "LaunchAgents", "user_agent"),
        (Path("/Library/LaunchAgents"), "system_agent"),
        (Path("/Library/LaunchDaemons"), "system_daemon"),
    ]
    items = []
    for d, kind in dirs:
        if not d.is_dir():
            continue
        try:
            plists = sorted(d.glob("*.plist"))
        except PermissionError:
            ERRORS.append(f"권한 없음: {d}")
            continue
        for pl in plists:
            entry = {"file": str(pl), "kind": kind, "label": None, "program": None, "run_at_load": None}
            try:
                with open(pl, "rb") as fp:
                    data = plistlib.load(fp)
                entry["label"] = data.get("Label")
                prog = data.get("Program") or (data.get("ProgramArguments") or [None])[0]
                entry["program"] = prog
                entry["run_at_load"] = data.get("RunAtLoad")
            except Exception as e:  # noqa: BLE001
                entry["parse_error"] = str(e)[:120]
            items.append(entry)
    return items


# ---------------------------------------------------------------- 요약 출력

def print_summary(report: dict) -> None:
    s = report["sections"]
    line = "─" * 62
    print(f"\n{line}\n 📦 macOS 설치 인벤토리 요약  ({report['generated_at'][:19]})\n{line}")

    apps = s.get("apps") or []
    print(f"  GUI 앱               : {len(apps)}개")

    brew = s.get("brew")
    if brew:
        leaves = [f for f in brew["formulae"] if f["installed_on_request"]]
        deps = len(brew["formulae"]) - len(leaves)
        print(f"  brew formulae        : {len(brew['formulae'])}개 (직접 설치 {len(leaves)} / 의존성 {deps})")
        print(f"  brew casks           : {len(brew['casks'])}개")
    else:
        print("  brew                 : 미설치")

    npm = s.get("npm")
    print(f"  npm 글로벌           : {len(npm['packages'])}개" if npm else "  npm                  : 미설치")

    for key, label in (("pipx", "pipx"), ("cargo", "cargo"), ("go_bin", "~/go/bin")):
        v = s.get(key)
        if v is not None:
            print(f"  {label:<20} : {len(v)}개")

    mas = s.get("mas")
    if mas:
        print(f"  App Store (mas)      : {len(mas['apps'])}개")

    ld = s.get("launchd") or []
    print(f"  LaunchAgents/Daemons : {len(ld)}개")

    # 업데이트 대기 목록
    outdated_rows = []
    if brew:
        for f in brew["formulae"]:
            if f["outdated"]:
                outdated_rows.append(("brew", f["name"], f["version"], f["latest_version"]))
        for c in brew["casks"]:
            if c["outdated"]:
                outdated_rows.append(("cask", c["name"], c["version"], c["latest_version"]))
    if npm:
        for p in npm["packages"]:
            if p["outdated"]:
                outdated_rows.append(("npm", p["name"], p["version"], p["latest_version"]))
    if mas:
        for o in mas["outdated"]:
            outdated_rows.append(("mas", o["name"], o["version"], o["latest_version"]))

    if outdated_rows:
        print(f"\n{line}\n ⬆️  업데이트 대기 중 ({len(outdated_rows)}개)\n{line}")
        for src, name, cur, latest in sorted(outdated_rows):
            print(f"  [{src:<4}] {name:<30} {str(cur):<15} → {latest}")
    elif not report.get("outdated_skipped"):
        print("\n  ✅ 모든 패키지가 최신 상태입니다.")

    # 용량 상위 앱
    sized = [a for a in apps if a.get("size_kb")]
    if sized:
        print(f"\n{line}\n 💾 용량 큰 앱 Top 10\n{line}")
        for a in sorted(sized, key=lambda x: -x["size_kb"])[:10]:
            print(f"  {human_size(a['size_kb']):>9}  {a['name']}")

    if ERRORS:
        print(f"\n  ⚠️  경고 {len(ERRORS)}건 (상세는 JSON errors 필드 참고)")
    print()


# ---------------------------------------------------------------- main

def main() -> int:
    ap = argparse.ArgumentParser(description="macOS 설치 소프트웨어 전체 인벤토리 스캐너")
    ap.add_argument("-o", "--output", default=None, help="JSON 출력 경로")
    ap.add_argument("--no-outdated", action="store_true", help="최신버전 체크 생략 (네트워크 조회 없음, 빠름)")
    ap.add_argument("--no-sizes", action="store_true", help="앱 용량(du) 계산 생략")
    ap.add_argument("--quiet", action="store_true", help="요약 출력 생략")
    args = ap.parse_args()

    if sys.platform != "darwin":
        print("⚠️  macOS가 아닌 환경입니다. mdls/brew 등 일부 스캔은 건너뜁니다.", file=sys.stderr)

    def step(msg):
        if not args.quiet:
            print(f"  … {msg}", file=sys.stderr)

    step("GUI 앱 스캔 중")
    apps = scan_apps(with_size=not args.no_sizes) if sys.platform == "darwin" else []

    step("Homebrew 조회 중")
    brew = scan_brew()

    step("npm 글로벌 조회 중")
    npm = scan_npm()

    step("pipx / cargo / mas / go 조회 중")
    pipx = scan_pipx()
    cargo = scan_cargo()
    mas = scan_mas()
    go_bin = scan_go_bin()

    step("LaunchAgents / LaunchDaemons 스캔 중")
    launchd = scan_launchd()

    if not args.no_outdated:
        if brew:
            step("brew 최신버전 확인 중 (수십 초 걸릴 수 있음)")
            check_brew_outdated(brew)
        if npm:
            step("npm 최신버전 확인 중")
            check_npm_outdated(npm)

    report = {
        "generated_at": datetime.now(timezone.utc).astimezone().isoformat(),
        "hostname": os.uname().nodename,
        "outdated_skipped": args.no_outdated,
        "sections": {
            "apps": apps,
            "brew": brew,
            "npm": npm,
            "pipx": pipx,
            "cargo": cargo,
            "mas": mas,
            "go_bin": go_bin,
            "launchd": launchd,
        },
        "errors": ERRORS,
    }

    out_path = Path(args.output or f"mac_inventory_{datetime.now():%Y%m%d_%H%M%S}.json")
    out_path.write_text(json.dumps(report, ensure_ascii=False, indent=2))

    if not args.quiet:
        print_summary(report)
    print(f"📄 상세 리포트: {out_path.resolve()}")
    return 0


if __name__ == "__main__":
    sys.exit(main())