package kafkaaccel

import (
	"fmt"
	"strings"
	"time"

	"github.com/luuuunet/owpanel/internal/models"
)

type AutoEnableResult struct {
	Enabled           bool         `json:"enabled"`
	NeedsKafkaInstall bool         `json:"needs_kafka_install"`
	InstallJobKey     string       `json:"install_job_key,omitempty"`
	LinkedDatabaseIDs []uint       `json:"linked_database_ids"`
	Steps             []string     `json:"steps"`
	Apply             *ApplyResult `json:"apply,omitempty"`
	Message           string       `json:"message"`
}

func (s *Service) isDatabaseEligible(inst *models.DatabaseInstance) bool {
	if inst == nil {
		return false
	}
	t := strings.ToLower(strings.TrimSpace(inst.Type))
	if t == "" {
		t = "mysql"
	}
	switch t {
	case "mysql", "mariadb", "postgresql", "postgres":
	default:
		return false
	}
	host := strings.ToLower(strings.TrimSpace(inst.Host))
	return host == "" || host == "127.0.0.1" || host == "localhost" || host == "::1"
}

func (s *Service) eligibleDatabaseIDs() ([]uint, error) {
	var list []models.DatabaseInstance
	if err := s.db.Find(&list).Error; err != nil {
		return nil, err
	}
	out := make([]uint, 0, len(list))
	for i := range list {
		if s.isDatabaseEligible(&list[i]) {
			out = append(out, list[i].ID)
		}
	}
	return out, nil
}

func (s *Service) validateEligibleDatabaseID(id uint) (*models.DatabaseInstance, error) {
	var inst models.DatabaseInstance
	if err := s.db.First(&inst, id).Error; err != nil {
		return nil, fmt.Errorf("数据库不存在")
	}
	if !s.isDatabaseEligible(&inst) {
		return nil, fmt.Errorf("仅支持本地 MySQL / PostgreSQL 数据库接入加速")
	}
	return &inst, nil
}

func mergeLinkedIDs(existing, add []uint) []uint {
	seen := map[uint]struct{}{}
	out := make([]uint, 0, len(existing)+len(add))
	for _, id := range existing {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	for _, id := range add {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func (s *Service) waitForBroker(bootstrap string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if s.kafkaRunning() && s.brokerReachable(bootstrap) {
			return true
		}
		time.Sleep(2 * time.Second)
	}
	return s.kafkaRunning() && s.brokerReachable(bootstrap)
}

func (s *Service) ensureKafkaReady(steps *[]string) error {
	cfg, err := s.GetConfig()
	if err != nil {
		return err
	}
	bootstrap := strings.TrimSpace(cfg.BootstrapServers)
	if bootstrap == "" {
		bootstrap = "127.0.0.1:9092"
	}
	if s.kafkaRunning() && s.brokerReachable(bootstrap) {
		*steps = append(*steps, "Kafka 已运行")
		return nil
	}
	if s.apps == nil {
		return fmt.Errorf("应用商店不可用")
	}

	if s.kafkaInstalled() || s.containerRunning(defaultContainer) {
		*steps = append(*steps, "正在启动 Kafka …")
		_ = s.apps.ServiceAction(kafkaAppKey, "start")
		if s.waitForBroker(bootstrap, 90*time.Second) {
			*steps = append(*steps, "Kafka 已启动")
			return nil
		}
	}

	if !s.kafkaInstalled() && !s.containerRunning(defaultContainer) {
		return fmt.Errorf("needs_kafka_install")
	}
	return fmt.Errorf("Kafka 已安装但 Broker 不可达，请检查 Docker 容器 owpanel-kafka")
}

func (s *Service) AutoEnableForDatabase(databaseID uint, installKafka bool) (*AutoEnableResult, error) {
	inst, err := s.validateEligibleDatabaseID(databaseID)
	if err != nil {
		return nil, err
	}
	return s.autoEnableInternal(installKafka, []uint{databaseID}, fmt.Sprintf("数据库 %s", inst.Name))
}

func (s *Service) AutoEnable(installKafka bool) (*AutoEnableResult, error) {
	return s.autoEnableInternal(installKafka, nil, "")
}

func (s *Service) autoEnableInternal(installKafka bool, targetIDs []uint, singleLabel string) (*AutoEnableResult, error) {
	steps := []string{"检测本地 MySQL / PostgreSQL 数据库 …"}
	var ids []uint
	var err error
	if len(targetIDs) > 0 {
		ids = append([]uint(nil), targetIDs...)
		if singleLabel != "" {
			steps = append(steps, fmt.Sprintf("目标: %s", singleLabel))
		}
	} else {
		ids, err = s.eligibleDatabaseIDs()
		if err != nil {
			return nil, err
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("未找到可加速的本地数据库，请先添加 MySQL 或 PostgreSQL")
	}
	if singleLabel == "" {
		steps = append(steps, fmt.Sprintf("发现 %d 个可加速数据库", len(ids)))
	}

	cfg, err := s.GetConfig()
	if err != nil {
		return nil, err
	}
	bootstrap := strings.TrimSpace(cfg.BootstrapServers)
	if bootstrap == "" {
		bootstrap = "127.0.0.1:9092"
	}

	if !s.kafkaRunning() || !s.brokerReachable(bootstrap) {
		if !installKafka {
			return &AutoEnableResult{
				NeedsKafkaInstall: true,
				InstallJobKey:     kafkaAppKey,
				LinkedDatabaseIDs: ids,
				Steps:             steps,
				Message:           "需要先安装并启动 Kafka",
			}, nil
		}
		steps = append(steps, "检测 Kafka …")
		if err := s.ensureKafkaReady(&steps); err != nil {
			if err.Error() == "needs_kafka_install" {
				steps = append(steps, "正在安装 Kafka（含 Docker 依赖）…")
				if installErr := s.apps.Install(kafkaAppKey, "latest"); installErr != nil && !strings.Contains(installErr.Error(), "already") && !strings.Contains(installErr.Error(), "in progress") {
					return nil, installErr
				}
				if waitErr := s.apps.WaitInstall(kafkaAppKey, 20*time.Minute); waitErr != nil {
					return nil, fmt.Errorf("Kafka 安装失败: %w", waitErr)
				}
				steps = append(steps, "Kafka 安装完成")
			} else {
				return nil, err
			}
		}
		if !s.waitForBroker(bootstrap, 120*time.Second) {
			return nil, fmt.Errorf("Kafka 启动后 Broker 仍不可达 (%s)", bootstrap)
		}
	}

	patch := *cfg
	patch.Enabled = true
	patch.LinkedDatabaseIDs = FormatLinkedIDs(mergeLinkedIDs(ParseLinkedIDs(cfg.LinkedDatabaseIDs), ids))
	if strings.TrimSpace(patch.Mode) == "" {
		patch.Mode = "write_async"
	}
	if _, err := s.UpdateConfig(&patch); err != nil {
		return nil, err
	}
	if singleLabel != "" {
		steps = append(steps, fmt.Sprintf("已将 %s 接入 Kafka 加速", singleLabel))
	} else {
		steps = append(steps, "已启用数据库加速并绑定全部本地库")
	}

	apply, err := s.Apply()
	if err != nil {
		return nil, fmt.Errorf("创建 Topic 失败: %w", err)
	}
	steps = append(steps, apply.Message)

	linked := ParseLinkedIDs(patch.LinkedDatabaseIDs)
	msg := fmt.Sprintf("已为 %d 个数据库开启 Kafka 加速", len(linked))
	if singleLabel != "" {
		msg = fmt.Sprintf("数据库 %s 已接入 Kafka 加速", strings.TrimPrefix(singleLabel, "数据库 "))
	}

	return &AutoEnableResult{
		Enabled:           true,
		LinkedDatabaseIDs: linked,
		Steps:             steps,
		Apply:             apply,
		Message:           msg,
	}, nil
}

func (s *Service) AccelSummaryForDatabase(dbID uint) (enabled bool, accelerated bool) {
	cfg, err := s.GetConfig()
	if err != nil || cfg == nil || !cfg.Enabled {
		return false, false
	}
	for _, id := range ParseLinkedIDs(cfg.LinkedDatabaseIDs) {
		if id == dbID {
			return true, true
		}
	}
	return true, false
}
