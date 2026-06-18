package cron

import "fmt"

type Template struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Desc     string `json:"description"`
	Schedule string `json:"schedule"`
	Command  string `json:"command"`
	Icon     string `json:"icon"`
	Color    string `json:"color"`
}

func (s *Service) Templates(lang string) []Template {
	data := s.dataDir
	return []Template{
		{ID: "docker-prune", Name: t(lang, "dockerPrune"), Desc: t(lang, "dockerPruneDesc"), Schedule: "0 3 * * *", Command: "docker system prune -f 2>/dev/null || true", Icon: "D", Color: "#2496ed"},
		{ID: "nginx-reload", Name: t(lang, "nginxReload"), Desc: t(lang, "nginxReloadDesc"), Schedule: "0 4 * * *", Command: "(nginx -t && systemctl reload nginx) 2>/dev/null || (openresty -t && systemctl reload openresty) 2>/dev/null || true", Icon: "N", Color: "#009639"},
		{ID: "certbot", Name: t(lang, "certbot"), Desc: t(lang, "certbotDesc"), Schedule: "0 2 * * *", Command: "certbot renew --quiet 2>/dev/null && systemctl reload nginx 2>/dev/null || true", Icon: "SSL", Color: "#f59e0b"},
		{ID: "disk-clean", Name: t(lang, "diskClean"), Desc: t(lang, "diskCleanDesc"), Schedule: "0 1 * * 0", Command: `find /tmp -type f -mtime +7 -delete 2>/dev/null; find /var/log -name "*.gz" -mtime +30 -delete 2>/dev/null || true`, Icon: "CL", Color: "#6366f1"},
		{ID: "backup-panel", Name: t(lang, "backupPanel"), Desc: t(lang, "backupPanelDesc"), Schedule: "0 3 * * *", Command: "tar -czf /opt/owpanel-backup-$(date +\\%Y\\%m\\%d).tar.gz -C /opt owpanel 2>/dev/null || true", Icon: "BK", Color: "#8b5cf6"},
		{ID: "mysql-dump", Name: t(lang, "mysqlDump"), Desc: t(lang, "mysqlDumpDesc"), Schedule: "0 2 * * *", Command: "mysqldump --all-databases > /opt/mysql-backup-$(date +\\%Y\\%m\\%d).sql 2>/dev/null || mariadb-dump --all-databases > /opt/mysql-backup-$(date +\\%Y\\%m\\%d).sql 2>/dev/null || true", Icon: "DB", Color: "#00758f"},
		{ID: "free-memory", Name: t(lang, "freeMemory"), Desc: t(lang, "freeMemoryDesc"), Schedule: "30 4 * * *", Command: `echo "=== before ==="; free -h; sync; echo 3 > /proc/sys/vm/drop_caches 2>/dev/null; sleep 1; echo "=== after ==="; free -h`, Icon: "RAM", Color: "#ec4899"},
		{ID: "mysql-optimize", Name: t(lang, "mysqlOptimize"), Desc: t(lang, "mysqlOptimizeDesc"), Schedule: "0 5 * * 0", Command: "mysqlcheck -o --all-databases 2>/dev/null || mariadb-check -o --all-databases 2>/dev/null || true", Icon: "DB+", Color: "#0d9488"},
		{ID: "panel-log-clean", Name: t(lang, "panelLogClean"), Desc: t(lang, "panelLogCleanDesc"), Schedule: "0 1 * * *", Command: panelLogCleanCmd(data), Icon: "LG", Color: "#64748b"},
		{ID: "restart-docker", Name: t(lang, "restartDocker"), Desc: t(lang, "restartDockerDesc"), Schedule: "0 5 * * 0", Command: "systemctl restart docker 2>/dev/null || service docker restart 2>/dev/null || true", Icon: "RD", Color: "#2496ed"},
		{ID: "swap-clear", Name: t(lang, "swapClear"), Desc: t(lang, "swapClearDesc"), Schedule: "0 6 * * *", Command: `used=$(free -m | awk "/Swap:/{print \$3}"); if [ "${used:-0}" -gt 512 ] 2>/dev/null; then sync; echo 3 > /proc/sys/vm/drop_caches 2>/dev/null; swapoff -a 2>/dev/null && swapon -a 2>/dev/null; fi`, Icon: "SW", Color: "#a855f7"},
	}
}

func panelLogCleanCmd(dataDir string) string {
	return fmt.Sprintf(`find %s/logs -name "*.gz" -mtime +14 -delete 2>/dev/null; find %s/cron/logs -name "*.log" -mtime +30 -delete 2>/dev/null; find %s/logs -name "*.log" -size +100M -exec truncate -s 0 {} \; 2>/dev/null || true`,
		dataDir, dataDir, dataDir)
}

func t(lang, key string) string {
	zh := map[string]string{
		"dockerPrune": "Docker 清理", "dockerPruneDesc": "清理未使用的镜像与容器",
		"nginxReload": "Nginx 重载", "nginxReloadDesc": "检测配置并重载 Web 服务",
		"certbot": "SSL 续期", "certbotDesc": "Certbot 自动续期证书",
		"diskClean": "磁盘清理", "diskCleanDesc": "清理临时文件与旧日志",
		"backupPanel": "面板备份", "backupPanelDesc": "打包备份 /opt/owpanel",
		"mysqlDump": "MySQL 备份", "mysqlDumpDesc": "导出全部数据库",
		"freeMemory": "释放内存", "freeMemoryDesc": "清理页面缓存，与仪表盘一键释放相同",
		"mysqlOptimize": "MySQL 优化", "mysqlOptimizeDesc": "优化全部数据表",
		"panelLogClean": "面板日志清理", "panelLogCleanDesc": "清理过期日志并截断超大日志",
		"restartDocker": "重启 Docker", "restartDockerDesc": "每周重启 Docker 服务",
		"swapClear": "Swap 整理", "swapClearDesc": "Swap 占用过高时刷新",
	}
	en := map[string]string{
		"dockerPrune": "Docker prune", "dockerPruneDesc": "Remove unused images and containers",
		"nginxReload": "Nginx reload", "nginxReloadDesc": "Test config and reload web server",
		"certbot": "SSL renew", "certbotDesc": "Certbot auto-renew certificates",
		"diskClean": "Disk cleanup", "diskCleanDesc": "Clean temp files and old logs",
		"backupPanel": "Panel backup", "backupPanelDesc": "Tar backup of /opt/owpanel",
		"mysqlDump": "MySQL backup", "mysqlDumpDesc": "Dump all databases",
		"freeMemory": "Free memory", "freeMemoryDesc": "Drop page cache like dashboard button",
		"mysqlOptimize": "MySQL optimize", "mysqlOptimizeDesc": "Optimize all tables",
		"panelLogClean": "Panel log cleanup", "panelLogCleanDesc": "Remove old logs and truncate large files",
		"restartDocker": "Restart Docker", "restartDockerDesc": "Weekly Docker restart",
		"swapClear": "Swap reclaim", "swapClearDesc": "Refresh swap when usage is high",
	}
	if lang == "en" {
		if v, ok := en[key]; ok {
			return v
		}
	}
	if v, ok := zh[key]; ok {
		return v
	}
	return key
}
