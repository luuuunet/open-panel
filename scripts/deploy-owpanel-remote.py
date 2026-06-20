"""Deploy owpanel binary + web assets to remote server."""
import os
import paramiko

HOST = "198.199.120.139"
USER = "root"
PASSWORD = "Wuyfieng0Wuyifeng"
BINARY = r"C:\Users\Administrator\Projects\open-panel\dist-fix-owpanel"
WEB_LOCAL = r"C:\Users\Administrator\Projects\open-panel\backend\web"
WEB_REMOTE = "/opt/owpanel/web"


def run(ssh, cmd, timeout=300):
    print(">>>", cmd[:160])
    _, stdout, stderr = ssh.exec_command(cmd, timeout=timeout)
    out = stdout.read().decode("utf-8", "replace")
    err = stderr.read().decode("utf-8", "replace")
    if out.strip():
        print(out[:2000])
    if err.strip():
        print("ERR:", err[:2000])


def upload_dir(sftp, local, remote):
    for root, dirs, files in os.walk(local):
        rel = os.path.relpath(root, local).replace("\\", "/")
        rdir = remote if rel == "." else f"{remote}/{rel}"
        try:
            sftp.mkdir(rdir)
        except OSError:
            pass
        for f in files:
            lp = os.path.join(root, f)
            rp = f"{rdir}/{f}"
            sftp.put(lp, rp)


def main():
    ssh = paramiko.SSHClient()
    ssh.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    ssh.connect(HOST, username=USER, password=PASSWORD, timeout=20)
    sftp = ssh.open_sftp()
    try:
        sftp.put(BINARY, "/opt/owpanel/owpanel.new")
        run(ssh, "chmod +x /opt/owpanel/owpanel.new && cp /opt/owpanel/owpanel /opt/owpanel/owpanel.bak 2>/dev/null; mv /opt/owpanel/owpanel.new /opt/owpanel/owpanel")
        print("uploading web assets...")
        upload_dir(sftp, WEB_LOCAL, WEB_REMOTE)
        run(ssh, "systemctl restart owpanel && sleep 2 && systemctl is-active owpanel")
    finally:
        sftp.close()
        ssh.close()


if __name__ == "__main__":
    main()
