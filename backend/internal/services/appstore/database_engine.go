package appstore

import (
	"strings"
)

// DatabaseEngineStatus describes panel-managed database engine readiness.
type DatabaseEngineStatus struct {
	Type        string `json:"type"`
	SoftwareKey string `json:"software_key"`
	Name        string `json:"name"`
	Installed   bool   `json:"installed"`
	Installing  bool   `json:"installing"`
	Running     bool   `json:"running"`
	Version     string `json:"version,omitempty"`
}

func databaseEngineSoftwareKeys(engineType string) []string {
	switch strings.ToLower(strings.TrimSpace(engineType)) {
	case "mysql", "mariadb":
		return []string{"mysql", "mariadb"}
	case "postgresql", "postgres", "pgsql":
		return []string{"postgresql"}
	case "mongodb", "mongo":
		return []string{"mongodb"}
	case "redis":
		return []string{"redis"}
	default:
		return nil
	}
}

func (s *Service) DatabaseEngineStatus(engineType string) DatabaseEngineStatus {
	keys := databaseEngineSoftwareKeys(engineType)
	if len(keys) == 0 {
		return DatabaseEngineStatus{Type: engineType}
	}
	for _, key := range keys {
		if st := s.softwareEngineStatus(engineType, key); st.Installed {
			return st
		}
	}
	return s.softwareEngineStatus(engineType, keys[0])
}

func (s *Service) AllDatabaseEngineStatuses() map[string]DatabaseEngineStatus {
	types := []string{"mysql", "postgresql", "mongodb", "redis"}
	out := make(map[string]DatabaseEngineStatus, len(types))
	for _, typ := range types {
		out[typ] = s.DatabaseEngineStatus(typ)
	}
	return out
}

func (s *Service) DatabaseEngineInstalled(engineType string) bool {
	return s.DatabaseEngineStatus(engineType).Installed
}

func (s *Service) softwareEngineStatus(engineType, key string) DatabaseEngineStatus {
	st := DatabaseEngineStatus{Type: engineType, SoftwareKey: key}
	app, err := s.Get(key)
	if err != nil {
		return st
	}
	st.Name = app.Name
	st.Installing = app.Status == "installing"
	st.Installed = app.Installed && !IsSimulatedInstall(key, s.dataDir)
	if st.Version == "" {
		st.Version = app.Version
	}
	if st.Installed {
		live := s.detectAppStatus(key)
		st.Running = live == "running" || live == "simulated"
	}
	return st
}
