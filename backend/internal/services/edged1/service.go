package edged1

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
	"gorm.io/gorm"
)

var writeSQLBlocked = regexp.MustCompile(`(?i)\b(INSERT|UPDATE|DELETE|DROP|ALTER|CREATE|REPLACE|TRUNCATE|ATTACH|DETACH|PRAGMA)\b`)

type Service struct {
	db      *gorm.DB
	dataDir string
	d1Dir   string
}

type QueryResult struct {
	Columns []string        `json:"columns"`
	Rows    [][]interface{} `json:"rows"`
	RowCount int            `json:"row_count"`
}

func NewService(db *gorm.DB, dataDir string) *Service {
	d1Dir := filepath.Join(dataDir, "edge", "d1")
	_ = os.MkdirAll(d1Dir, 0755)
	return &Service{db: db, dataDir: dataDir, d1Dir: d1Dir}
}

func (s *Service) List() ([]models.EdgeD1Database, error) {
	var list []models.EdgeD1Database
	return list, s.db.Order("id asc").Find(&list).Error
}

func (s *Service) Get(id uint) (*models.EdgeD1Database, error) {
	var db models.EdgeD1Database
	if err := s.db.First(&db, id).Error; err != nil {
		return nil, err
	}
	return &db, nil
}

func (s *Service) Create(name, description string) (*models.EdgeD1Database, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("database name is required")
	}
	row := models.EdgeD1Database{Name: name, Description: description}
	if err := s.db.Create(&row).Error; err != nil {
		return nil, err
	}
	filePath := filepath.Join(s.d1Dir, fmt.Sprintf("%d.sqlite", row.ID))
	if err := s.initSQLiteFile(filePath); err != nil {
		_ = s.db.Delete(&row).Error
		return nil, err
	}
	row.FilePath = filePath
	if err := s.db.Save(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *Service) Update(id uint, name, description string) error {
	db, err := s.Get(id)
	if err != nil {
		return err
	}
	if name != "" {
		db.Name = strings.TrimSpace(name)
	}
	db.Description = description
	return s.db.Save(db).Error
}

func (s *Service) Delete(id uint) error {
	db, err := s.Get(id)
	if err != nil {
		return err
	}
	if db.FilePath != "" {
		_ = os.Remove(db.FilePath)
	}
	return s.db.Delete(&models.EdgeD1Database{}, id).Error
}

func (s *Service) Query(id uint, sqlText string, readOnly bool) (*QueryResult, error) {
	db, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	sqlText = strings.TrimSpace(sqlText)
	if sqlText == "" {
		return nil, fmt.Errorf("SQL is required")
	}
	if readOnly {
		if writeSQLBlocked.MatchString(sqlText) {
			return nil, fmt.Errorf("only SELECT queries are allowed in worker context")
		}
		if !strings.HasPrefix(strings.ToUpper(sqlText), "SELECT") &&
			!strings.HasPrefix(strings.ToUpper(sqlText), "WITH") &&
			!strings.HasPrefix(strings.ToUpper(sqlText), "EXPLAIN") {
			return nil, fmt.Errorf("only read-only SELECT queries are allowed")
		}
	}
	return s.runQuery(db.FilePath, sqlText)
}

func (s *Service) initSQLiteFile(path string) error {
	conn, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)")
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.Exec(`CREATE TABLE IF NOT EXISTS _edge_meta (key TEXT PRIMARY KEY, value TEXT)`)
	return err
}

func (s *Service) runQuery(dbPath, sqlText string) (*QueryResult, error) {
	if dbPath == "" {
		return nil, fmt.Errorf("database file missing")
	}
	conn, err := sql.Open("sqlite", dbPath+"?_pragma=query_only=1")
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	rows, err := conn.Query(sqlText)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	result := &QueryResult{Columns: cols, Rows: [][]interface{}{}}
	for rows.Next() {
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make([]interface{}, len(cols))
		for i, v := range vals {
			switch x := v.(type) {
			case []byte:
				row[i] = string(x)
			default:
				row[i] = x
			}
		}
		result.Rows = append(result.Rows, row)
	}
	result.RowCount = len(result.Rows)
	return result, nil
}

func (s *Service) ExecAdmin(id uint, sqlText string) (*QueryResult, error) {
	db, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	sqlText = strings.TrimSpace(sqlText)
	if sqlText == "" {
		return nil, fmt.Errorf("SQL is required")
	}
	upper := strings.ToUpper(strings.TrimSpace(sqlText))
	if strings.HasPrefix(upper, "SELECT") || strings.HasPrefix(upper, "WITH") || strings.HasPrefix(upper, "EXPLAIN") {
		return s.runQuery(db.FilePath, sqlText)
	}
	conn, err := sql.Open("sqlite", db.FilePath+"?_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	res, err := conn.Exec(sqlText)
	if err != nil {
		return nil, err
	}
	affected, _ := res.RowsAffected()
	return &QueryResult{
		Columns:  []string{"rows_affected"},
		Rows:     [][]interface{}{{affected}},
		RowCount: 1,
	}, nil
}
