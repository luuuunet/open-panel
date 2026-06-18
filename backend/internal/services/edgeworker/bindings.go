package edgeworker

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/luuuunet/owpanel/internal/models"
	"gorm.io/gorm"
)

var luaIdent = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

var allowedBindingTypes = map[string]bool{
	"kv": true, "d1": true, "redis": true, "oss": true,
}

type BindingInput struct {
	BindingType string `json:"binding_type"`
	BindingName string `json:"binding_name"`
	ResourceID  uint   `json:"resource_id"`
	ResourceKey string `json:"resource_key"`
}

func (s *Service) ListBindings(workerID uint) ([]models.EdgeWorkerBinding, error) {
	var list []models.EdgeWorkerBinding
	return list, s.db.Where("worker_id = ?", workerID).Order("id asc").Find(&list).Error
}

func (s *Service) SetBindings(workerID uint, inputs []BindingInput) error {
	if _, err := s.Get(workerID); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, in := range inputs {
		t := strings.ToLower(strings.TrimSpace(in.BindingType))
		name := strings.TrimSpace(in.BindingName)
		if !allowedBindingTypes[t] {
			return fmt.Errorf("invalid binding_type: %s", in.BindingType)
		}
		if !luaIdent.MatchString(name) {
			return fmt.Errorf("binding_name must be a valid Lua identifier: %s", name)
		}
		if seen[name] {
			return fmt.Errorf("duplicate binding_name: %s", name)
		}
		seen[name] = true
		if t == "kv" || t == "d1" || t == "oss" {
			if in.ResourceID == 0 {
				return fmt.Errorf("resource_id required for %s binding %s", t, name)
			}
		}
		if t == "redis" && strings.TrimSpace(in.ResourceKey) == "" {
			return fmt.Errorf("resource_key required for redis binding %s (host:port:db)", name)
		}
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("worker_id = ?", workerID).Delete(&models.EdgeWorkerBinding{}).Error; err != nil {
			return err
		}
		for _, in := range inputs {
			row := models.EdgeWorkerBinding{
				WorkerID:    workerID,
				BindingType: strings.ToLower(strings.TrimSpace(in.BindingType)),
				BindingName: strings.TrimSpace(in.BindingName),
				ResourceID:  in.ResourceID,
				ResourceKey: strings.TrimSpace(in.ResourceKey),
			}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Service) GenerateBindingPrelude(workerID uint) (string, error) {
	bindings, err := s.ListBindings(workerID)
	if err != nil || len(bindings) == 0 {
		return "", err
	}
	var b strings.Builder
	b.WriteString("local edge = require \"edge_runtime\"\n")
	for _, bind := range bindings {
		switch bind.BindingType {
		case "kv":
			b.WriteString(fmt.Sprintf("local %s = edge.kv(%d)\n", bind.BindingName, bind.ResourceID))
		case "d1":
			b.WriteString(fmt.Sprintf("local %s = edge.d1(%d)\n", bind.BindingName, bind.ResourceID))
		case "redis":
			key := strings.ReplaceAll(bind.ResourceKey, `"`, `\"`)
			b.WriteString(fmt.Sprintf("local %s = edge.redis(%q)\n", bind.BindingName, key))
		case "oss":
			b.WriteString(fmt.Sprintf("-- OSS binding %s -> storage id %d (use panel OSS module)\n", bind.BindingName, bind.ResourceID))
		}
	}
	b.WriteString("\n")
	return b.String(), nil
}

func (s *Service) CollectKVNamespaceIDs() ([]uint, error) {
	var ids []uint
	err := s.db.Model(&models.EdgeWorkerBinding{}).
		Where("binding_type = ?", "kv").
		Distinct("resource_id").
		Pluck("resource_id", &ids).Error
	return ids, err
}
