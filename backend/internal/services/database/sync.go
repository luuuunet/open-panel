package database

import (
	"fmt"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
)

var mysqlSystemDBs = map[string]struct{}{
	"information_schema": {},
	"mysql":              {},
	"performance_schema": {},
	"sys":                {},
}

var mysqlSystemUsers = map[string]struct{}{
	"root":              {},
	"mysql.sys":         {},
	"debian-sys-maint":  {},
	"mysql.session":     {},
	"mysql.infoschema":  {},
	"mysql.healthcheck": {},
}

type SyncResult struct {
	Added int `json:"added"`
}

// SyncFromServer imports databases that exist on local MySQL/PostgreSQL but are
// missing from the panel registry (e.g. created outside the panel).
func (s *Service) SyncFromServer() SyncResult {
	if s.db == nil {
		return SyncResult{}
	}
	r := SyncResult{}
	r.Added += s.syncMySQL()
	r.Added += s.syncPostgreSQL()
	r.Added += s.syncMongoDB()
	return r
}

var mongoSystemDBs = map[string]struct{}{
	"admin":  {},
	"local":  {},
	"config": {},
}

func (s *Service) syncMySQL() int {
	if _, err := findBinary("mysql", "mariadb"); err != nil {
		return 0
	}
	s.repairInvalidMySQLSyncRecords()
	out, err := s.mysqlRootExec("-e", "SHOW DATABASES;")
	if err != nil {
		return 0
	}
	added := 0
	for _, line := range strings.Split(string(out), "\n") {
		name := strings.TrimSpace(line)
		if !isValidMySQLSyncName(name) {
			continue
		}
		if _, ok := mysqlSystemDBs[strings.ToLower(name)]; ok {
			continue
		}
		if s.ensureMySQLRecord(name) {
			added++
		}
	}
	return added
}

func (s *Service) repairInvalidMySQLSyncRecords() {
	if s.db == nil {
		return
	}
	var list []models.DatabaseInstance
	s.db.Where("type IN ? AND remark = ?", []string{"mysql", "mariadb", ""}, "自动同步").Find(&list)
	for _, inst := range list {
		if !isValidMySQLSyncName(inst.Name) {
			_ = s.db.Delete(&models.DatabaseInstance{}, inst.ID).Error
			continue
		}
		if !isValidMySQLSyncName(inst.Username) {
			user := s.mysqlDbUser(inst.Name)
			if isValidMySQLSyncName(user) {
				_ = s.db.Model(&inst).Update("username", user).Error
			}
		}
	}
}

func (s *Service) ensureMySQLRecord(dbName string) bool {
	var count int64
	s.db.Model(&models.DatabaseInstance{}).
		Where("name = ? AND type IN ? AND host IN ?", dbName, []string{"mysql", "mariadb", ""}, localHosts()).
		Count(&count)
	if count > 0 {
		return false
	}
	user := s.mysqlDbUser(dbName)
	accessMode := s.detectMySQLAccessMode(user)
	srv := s.queryMySQLServer()
	dbType := srv.Engine
	if dbType == "" {
		dbType = detectMySQLEngine()
	}
	inst := &models.DatabaseInstance{
		Name:        dbName,
		Type:        dbType,
		Host:        "127.0.0.1",
		Port:        3306,
		Username:    user,
		Status:      "running",
		Remark:      "自动同步",
		AccessMode:  accessMode,
		AllowRemote: AllowRemoteFromAccessMode(accessMode),
	}
	return s.db.Create(inst).Error == nil
}

func (s *Service) mysqlDbUser(dbName string) string {
	esc := strings.ReplaceAll(dbName, "'", "''")
	q := fmt.Sprintf(
		"SELECT User FROM mysql.db WHERE Db='%s' AND User NOT IN ('root','mysql.sys','debian-sys-maint','mysql.session','mysql.infoschema','mysql.healthcheck') "+
			"ORDER BY CASE WHEN Host='localhost' THEN 0 WHEN Host='127.0.0.1' THEN 1 ELSE 2 END LIMIT 1;",
		esc,
	)
	out, err := s.mysqlRootExec("-e", q)
	if err == nil {
		if user := extractMySQLUserFromOutput(string(out)); user != "" {
			if _, skip := mysqlSystemUsers[user]; !skip {
				return user
			}
		}
	}
	if user := strings.TrimSpace(dbName); user != "" {
		return user
	}
	return dbName
}

func (s *Service) syncPostgreSQL() int {
	if _, err := findBinary("psql"); err != nil {
		return 0
	}
	out, err := s.pgSuperExec("-tAc", "SELECT datname FROM pg_database WHERE datallowconn AND NOT datistemplate AND datname NOT IN ('postgres');")
	if err != nil {
		return 0
	}
	added := 0
	for _, line := range strings.Split(string(out), "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		if s.ensurePostgreSQLRecord(name) {
			added++
		}
	}
	return added
}

func (s *Service) ensurePostgreSQLRecord(dbName string) bool {
	var count int64
	s.db.Model(&models.DatabaseInstance{}).
		Where("name = ? AND type IN ? AND host IN ?", dbName, []string{"postgresql", "postgres"}, localHosts()).
		Count(&count)
	if count > 0 {
		return false
	}
	user := s.pgDbOwner(dbName)
	inst := &models.DatabaseInstance{
		Name:     dbName,
		Type:     "postgresql",
		Host:     "127.0.0.1",
		Port:     5432,
		Username: user,
		Status:   "running",
		Remark:   "自动同步",
	}
	return s.db.Create(inst).Error == nil
}

func (s *Service) pgDbOwner(dbName string) string {
	esc := strings.ReplaceAll(dbName, "'", "''")
	q := fmt.Sprintf("SELECT pg_catalog.pg_get_userbyid(d.datdba) FROM pg_catalog.pg_database d WHERE d.datname = '%s';", esc)
	out, err := s.pgSuperExec("-tAc", q)
	if err == nil {
		if user := strings.TrimSpace(string(out)); user != "" {
			return user
		}
	}
	return "postgres"
}

func (s *Service) syncMongoDB() int {
	if _, err := findBinary("mongosh", "mongo"); err != nil {
		return 0
	}
	out, err := s.mongoShellExec("--eval", "db.adminCommand({listDatabases:1}).databases.forEach(function(d){print(d.name)})")
	if err != nil {
		return 0
	}
	added := 0
	for _, line := range strings.Split(string(out), "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		if _, ok := mongoSystemDBs[strings.ToLower(name)]; ok {
			continue
		}
		if s.ensureMongoDBRecord(name) {
			added++
		}
	}
	return added
}

func (s *Service) ensureMongoDBRecord(dbName string) bool {
	var count int64
	s.db.Model(&models.DatabaseInstance{}).
		Where("name = ? AND type = ? AND host IN ?", dbName, "mongodb", localHosts()).
		Count(&count)
	if count > 0 {
		return false
	}
	inst := &models.DatabaseInstance{
		Name:   dbName,
		Type:   "mongodb",
		Host:   "127.0.0.1",
		Port:   27017,
		Status: "running",
		Remark: "自动同步",
	}
	return s.db.Create(inst).Error == nil
}

func localHosts() []string {
	return []string{"127.0.0.1", "localhost", "::1", ""}
}
