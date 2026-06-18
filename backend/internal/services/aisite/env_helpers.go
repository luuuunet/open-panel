package aisite

import "github.com/luuuunet/owpanel/internal/services/appstore"

func frameworkHint(plan DeployPlan, snap *RepoSnapshot) string {
	if plan.Framework != "" {
		return plan.Framework
	}
	if snap != nil {
		return snap.FrameworkHint
	}
	return ""
}

func deployAppReady(s *Service, key string) bool {
	switch key {
	case "composer":
		return s.envComposerReady()
	case "nodejs20":
		return appstore.NodeMajorAvailable(s.dataDir, 20)
	case "nodejs18":
		return appstore.NodeMajorAvailable(s.dataDir, 18) || appstore.NodeMajorAvailable(s.dataDir, 20)
	case "mysql", "mariadb":
		return s.envMySQLReady()
	default:
		return s.isAppReady(key)
	}
}
