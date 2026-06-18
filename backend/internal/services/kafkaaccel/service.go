package kafkaaccel

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/luuuunet/owpanel/internal/services/appstore"
	"gorm.io/gorm"
)

const kafkaAppKey = "kafka"
const defaultContainer = "owpanel-kafka"

type Service struct {
	db   *gorm.DB
	apps *appstore.Service
}

func NewService(db *gorm.DB, apps *appstore.Service) *Service {
	return &Service{db: db, apps: apps}
}

func defaultConfig() KafkaAccelConfig {
	return KafkaAccelConfig{
		Scope:             "global",
		Enabled:           false,
		BootstrapServers:  "127.0.0.1:9092",
		TopicPrefix:       "opanel.db",
		Mode:              "write_async",
		ConsumerGroup:     "opanel-db-accel",
		TopicPartitions:   3,
		ReplicationFactor: 1,
		RetentionHours:    168,
		ProducerBatchSize: 32768,
		ProducerLingerMs:  5,
		CompressionType:   "lz4",
		FetchMinBytes:     1,
	}
}

func normalizeConfig(cfg *KafkaAccelConfig) {
	if cfg == nil {
		return
	}
	def := defaultConfig()
	if cfg.TopicPartitions <= 0 {
		cfg.TopicPartitions = def.TopicPartitions
	}
	if cfg.ReplicationFactor <= 0 {
		cfg.ReplicationFactor = def.ReplicationFactor
	}
	if cfg.RetentionHours <= 0 {
		cfg.RetentionHours = def.RetentionHours
	}
	if cfg.ProducerBatchSize <= 0 {
		cfg.ProducerBatchSize = def.ProducerBatchSize
	}
	if cfg.ProducerLingerMs < 0 {
		cfg.ProducerLingerMs = def.ProducerLingerMs
	}
	if strings.TrimSpace(cfg.CompressionType) == "" {
		cfg.CompressionType = def.CompressionType
	}
	if cfg.FetchMinBytes <= 0 {
		cfg.FetchMinBytes = def.FetchMinBytes
	}
}

func (s *Service) ensureDefaults() {
	var n int64
	s.db.Model(&KafkaAccelConfig{}).Where("scope = ?", "global").Count(&n)
	if n == 0 {
		cfg := defaultConfig()
		_ = s.db.Create(&cfg).Error
	}
}

func (s *Service) GetConfig() (*KafkaAccelConfig, error) {
	s.ensureDefaults()
	var cfg KafkaAccelConfig
	err := s.db.Where("scope = ?", "global").First(&cfg).Error
	if err == gorm.ErrRecordNotFound {
		cfg = defaultConfig()
		if err := s.db.Create(&cfg).Error; err != nil {
			return nil, err
		}
		return &cfg, nil
	}
	if err != nil {
		return nil, err
	}
	normalizeConfig(&cfg)
	return &cfg, err
}

func validateCompressionType(v string) error {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "none", "gzip", "snappy", "lz4", "zstd":
		return nil
	default:
		return fmt.Errorf("invalid compression_type: %s", v)
	}
}

func (s *Service) UpdateConfig(patch *KafkaAccelConfig) (*KafkaAccelConfig, error) {
	cfg, err := s.GetConfig()
	if err != nil {
		return nil, err
	}
	updates := map[string]interface{}{}
	if patch.Enabled != cfg.Enabled {
		updates["enabled"] = patch.Enabled
	}
	if v := strings.TrimSpace(patch.BootstrapServers); v != "" {
		updates["bootstrap_servers"] = v
	}
	if v := strings.TrimSpace(patch.TopicPrefix); v != "" {
		updates["topic_prefix"] = v
	}
	if v := strings.TrimSpace(patch.Mode); v != "" {
		switch v {
		case "write_async", "cache_invalidate", "read_through":
			updates["mode"] = v
		default:
			return nil, fmt.Errorf("invalid mode: %s", v)
		}
	}
	updates["linked_database_ids"] = strings.TrimSpace(patch.LinkedDatabaseIDs)
	if v := strings.TrimSpace(patch.ConsumerGroup); v != "" {
		updates["consumer_group"] = v
	}
	if patch.TopicPartitions > 0 && patch.TopicPartitions != cfg.TopicPartitions {
		if patch.TopicPartitions > 100 {
			return nil, fmt.Errorf("topic_partitions must be 1-100")
		}
		updates["topic_partitions"] = patch.TopicPartitions
	}
	if patch.ReplicationFactor > 0 && patch.ReplicationFactor != cfg.ReplicationFactor {
		if patch.ReplicationFactor > 10 {
			return nil, fmt.Errorf("replication_factor must be 1-10")
		}
		updates["replication_factor"] = patch.ReplicationFactor
	}
	if patch.RetentionHours > 0 && patch.RetentionHours != cfg.RetentionHours {
		if patch.RetentionHours > 8760 {
			return nil, fmt.Errorf("retention_hours must be 1-8760")
		}
		updates["retention_hours"] = patch.RetentionHours
	}
	if patch.ProducerBatchSize > 0 && patch.ProducerBatchSize != cfg.ProducerBatchSize {
		if patch.ProducerBatchSize < 1024 || patch.ProducerBatchSize > 1048576 {
			return nil, fmt.Errorf("producer_batch_size must be 1024-1048576")
		}
		updates["producer_batch_size"] = patch.ProducerBatchSize
	}
	if patch.ProducerLingerMs != cfg.ProducerLingerMs {
		if patch.ProducerLingerMs < 0 || patch.ProducerLingerMs > 1000 {
			return nil, fmt.Errorf("producer_linger_ms must be 0-1000")
		}
		updates["producer_linger_ms"] = patch.ProducerLingerMs
	}
	if v := strings.TrimSpace(patch.CompressionType); v != "" && !strings.EqualFold(v, cfg.CompressionType) {
		if err := validateCompressionType(v); err != nil {
			return nil, err
		}
		updates["compression_type"] = strings.ToLower(v)
	}
	if patch.FetchMinBytes > 0 && patch.FetchMinBytes != cfg.FetchMinBytes {
		if patch.FetchMinBytes > 1048576 {
			return nil, fmt.Errorf("fetch_min_bytes must be 1-1048576")
		}
		updates["fetch_min_bytes"] = patch.FetchMinBytes
	}
	if len(updates) == 0 {
		return cfg, nil
	}
	if err := s.db.Model(cfg).Updates(updates).Error; err != nil {
		return nil, err
	}
	return s.GetConfig()
}

func ParseLinkedIDs(raw string) []uint {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]uint, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.ParseUint(p, 10, 64)
		if err != nil || n == 0 {
			continue
		}
		out = append(out, uint(n))
	}
	return out
}

func FormatLinkedIDs(ids []uint) string {
	if len(ids) == 0 {
		return ""
	}
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatUint(uint64(id), 10)
	}
	return strings.Join(parts, ",")
}

func (s *Service) writeTopicName(prefix string, dbID uint, suffix string) string {
	prefix = strings.Trim(strings.TrimSpace(prefix), ".")
	if prefix == "" {
		prefix = "opanel.db"
	}
	return fmt.Sprintf("%s.%d.%s", prefix, dbID, suffix)
}

func (s *Service) ExpectedTopics(cfg *KafkaAccelConfig) []string {
	if cfg == nil {
		return nil
	}
	ids := ParseLinkedIDs(cfg.LinkedDatabaseIDs)
	topics := make([]string, 0, len(ids)*2)
	for _, id := range ids {
		topics = append(topics,
			s.writeTopicName(cfg.TopicPrefix, id, "writes"),
			s.writeTopicName(cfg.TopicPrefix, id, "invalidate"),
		)
	}
	return topics
}
