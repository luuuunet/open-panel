#!/usr/bin/env python3
"""Build-aware deploy for OWPanel (binary + web) via SSH/SFTP."""
from __future__ import annotations

import argparse
import io
import os
import sys
import tarfile
from pathlib import Path

try:
    import paramiko
except ImportError:
    print("Install paramiko: pip install paramiko", file=sys.stderr)
    sys.exit(1)

ROOT = Path(__file__).resolve().parent.parent
ENV_FILE = Path(__file__).resolve().parent / "deploy.env"


def load_env() -> None:
    if not ENV_FILE.is_file():
        return
    for line in ENV_FILE.read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, _, val = line.partition("=")
        key, val = key.strip(), val.strip().strip('"').strip("'")
        os.environ.setdefault(key, val)


def cfg(name: str, default: str = "") -> str:
    return os.environ.get(name, default)


def log(msg: str) -> None:
    print(f"[deploy] {msg}", flush=True)


def connect() -> paramiko.SSHClient:
    host = cfg("DEPLOY_HOST")
    user = cfg("DEPLOY_USER", "root")
    password = cfg("DEPLOY_PASSWORD")
    key_path = os.path.expanduser(cfg("DEPLOY_KEY", ""))

    if not host:
        raise SystemExit("Set DEPLOY_HOST in environment or scripts/deploy.env")

    client = paramiko.SSHClient()
    client.set_missing_host_key_policy(paramiko.AutoAddPolicy())

    kwargs: dict = {"hostname": host, "username": user, "timeout": 30}
    if key_path and os.path.isfile(key_path):
        kwargs["key_filename"] = key_path
    elif password:
        kwargs["password"] = password
    else:
        raise SystemExit("Set DEPLOY_KEY or DEPLOY_PASSWORD in scripts/deploy.env")

    log(f"Connecting to {user}@{host}...")
    client.connect(**kwargs)
    return client


def run(client: paramiko.SSHClient, cmd: str, timeout: int = 600) -> int:
    log(f"$ {cmd[:180]}{'...' if len(cmd) > 180 else ''}")
    _, stdout, stderr = client.exec_command(cmd, timeout=timeout)
    out = stdout.read().decode("utf-8", errors="replace")
    err = stderr.read().decode("utf-8", errors="replace")
    code = stdout.channel.recv_exit_status()
    if out.strip():
        print(out.rstrip())
    if err.strip() and code != 0:
        print(err.rstrip(), file=sys.stderr)
    return code


def tarball_dir(src: Path, prefix: str = "") -> bytes:
    buf = io.BytesIO()
    with tarfile.open(fileobj=buf, mode="w:gz") as tar:
        for path in sorted(src.rglob("*")):
            if path.is_file():
                arc = path.relative_to(src).as_posix()
                if prefix:
                    arc = f"{prefix}/{arc}"
                tar.add(path, arcname=arc)
    return buf.getvalue()


def upload_bytes(client: paramiko.SSHClient, data: bytes, remote: str) -> None:
    log(f"Upload {len(data) / 1024 / 1024:.1f} MB -> {remote}")
    sftp = client.open_sftp()
    with sftp.file(remote, "wb") as rf:
        rf.write(data)
    sftp.close()


def upload_file(client: paramiko.SSHClient, local: Path, remote: str) -> None:
    log(f"Upload {local.name} ({local.stat().st_size / 1024 / 1024:.1f} MB)")
    sftp = client.open_sftp()
    sftp.put(str(local), remote)
    sftp.close()


def deploy_web(client: paramiko.SSHClient, install_dir: str, port: str) -> int:
    web = ROOT / "backend" / "web"
    if not web.is_dir():
        log(f"Missing {web} — run frontend build first")
        return 1

    remote_tar = "/tmp/owpanel-web.tar.gz"
    upload_bytes(client, tarball_dir(web), remote_tar)

    script = f"""
set -euo pipefail
rm -rf /tmp/owpanel-web-new && mkdir -p /tmp/owpanel-web-new
tar -xzf {remote_tar} -C /tmp/owpanel-web-new
rm -rf {install_dir}/web
cp -a /tmp/owpanel-web-new {install_dir}/web
systemctl restart owpanel
sleep 2
systemctl is-active owpanel
curl -sf -o /dev/null -w "HTTP %{{http_code}}\\n" "http://127.0.0.1:{port}/" || true
"""
    return run(client, f"bash -s <<'EOF'\n{script}\nEOF")


def deploy_stack(client: paramiko.SSHClient, install_dir: str) -> int:
    stack = ROOT / "scripts" / "stack"
    if not stack.is_dir():
        log(f"Missing {stack} — skip stack scripts upload")
        return 0
    remote_tar = "/tmp/owpanel-stack.tar.gz"
    upload_bytes(client, tarball_dir(stack), remote_tar)
    script = f"""
set -euo pipefail
rm -rf /tmp/owpanel-stack-new && mkdir -p /tmp/owpanel-stack-new
tar -xzf {remote_tar} -C /tmp/owpanel-stack-new
mkdir -p {install_dir}/scripts
rm -rf {install_dir}/scripts/stack
cp -a /tmp/owpanel-stack-new {install_dir}/scripts/stack
find {install_dir}/scripts/stack -name '*.sh' -exec chmod +x {{}} \\;
find {install_dir}/scripts/stack -name '*.sh' -exec sed -i 's/\\r$//' {{}} \\; 2>/dev/null || true
"""
    return run(client, f"bash -s <<'EOF'\n{script}\nEOF")


def deploy_binary(client: paramiko.SSHClient, install_dir: str, binary: Path) -> int:
    remote = "/tmp/owpanel-bin.new"
    upload_file(client, binary, remote)
    op = binary.parent / "op-linux-amd64"
    if not op.is_file():
        op = binary.parent / "op"
    script = f"""
set -euo pipefail
cp -f {remote} {install_dir}/owpanel
chmod +x {install_dir}/owpanel
"""
    if op.is_file():
        upload_file(client, op, "/tmp/op-bin.new")
        script += f"""
cp -f /tmp/op-bin.new {install_dir}/op
chmod +x {install_dir}/op
ln -sf {install_dir}/op /usr/local/bin/op
"""
    script += """
systemctl restart owpanel
sleep 2
systemctl is-active owpanel
"""
    return run(client, f"bash -s <<'EOF'\n{script}\nEOF")


def main() -> int:
    load_env()
    parser = argparse.ArgumentParser(description="Deploy OWPanel to remote server")
    parser.add_argument("--web-only", action="store_true", help="Upload frontend only")
    parser.add_argument("--binary", type=Path, help="Pre-built linux binary path")
    parser.add_argument("--full", action="store_true", help="Upload binary + web")
    args = parser.parse_args()

    install_dir = cfg("INSTALL_DIR", "/opt/owpanel")
    port = cfg("OWPANEL_PORT", "8888")

    client = connect()
    code = 0
    try:
        if args.web_only:
            code = deploy_web(client, install_dir, port)
        elif args.full or args.binary:
            binary = args.binary or ROOT / "dist" / "owpanel-linux-amd64" / "owpanel"
            if not binary.is_file():
                log(f"Binary not found: {binary}")
                return 1
            code = deploy_binary(client, install_dir, binary)
            if code == 0:
                code = deploy_stack(client, install_dir)
            if code == 0:
                code = deploy_web(client, install_dir, port)
        else:
            parser.print_help()
            return 1
    finally:
        client.close()

    if code == 0:
        host = cfg("DEPLOY_HOST")
        log(f"Done. Panel: http://{host}:{port}")
    return code


if __name__ == "__main__":
    sys.exit(main())
