package kafkaaccel

import (
	"fmt"
	"os/exec"
	"strings"
)

type TopicsResult struct {
	Topics []string `json:"topics"`
	Hint   string   `json:"hint,omitempty"`
}

type ApplyResult struct {
	Created []string `json:"created"`
	Skipped []string `json:"skipped,omitempty"`
	Message string   `json:"message"`
}

func (s *Service) ListTopics() (*TopicsResult, error) {
	cfg, err := s.GetConfig()
	if err != nil {
		return nil, err
	}
	if !s.kafkaRunning() {
		return &TopicsResult{
			Topics: nil,
			Hint:   "Start Kafka to list topics (App Store → Kafka, or docker start owpanel-kafka)",
		}, nil
	}
	topics, err := s.runKafkaTopicsList(cfg.BootstrapServers)
	if err != nil {
		return &TopicsResult{
			Topics: nil,
			Hint:   err.Error(),
		}, nil
	}
	return &TopicsResult{Topics: topics}, nil
}

func (s *Service) Apply() (*ApplyResult, error) {
	cfg, err := s.GetConfig()
	if err != nil {
		return nil, err
	}
	if !s.kafkaRunning() {
		return nil, fmt.Errorf("kafka is not running — install and start Kafka first")
	}
	ids := ParseLinkedIDs(cfg.LinkedDatabaseIDs)
	if len(ids) == 0 {
		return nil, fmt.Errorf("select at least one database to accelerate")
	}

	existing, _ := s.runKafkaTopicsList(cfg.BootstrapServers)
	existSet := map[string]struct{}{}
	for _, t := range existing {
		existSet[t] = struct{}{}
	}

	created := make([]string, 0)
	skipped := make([]string, 0)
	for _, id := range ids {
		for _, suffix := range []string{"writes", "invalidate"} {
			topic := s.writeTopicName(cfg.TopicPrefix, id, suffix)
			if _, ok := existSet[topic]; ok {
				skipped = append(skipped, topic)
				continue
			}
			if err := s.createTopic(cfg, topic); err != nil {
				return nil, fmt.Errorf("create topic %s: %w", topic, err)
			}
			created = append(created, topic)
			existSet[topic] = struct{}{}
		}
	}
	msg := fmt.Sprintf("Created %d topic(s)", len(created))
	if len(skipped) > 0 {
		msg += fmt.Sprintf(", %d already existed", len(skipped))
	}
	return &ApplyResult{Created: created, Skipped: skipped, Message: msg}, nil
}

func (s *Service) containerRunning(name string) bool {
	out, err := exec.Command("docker", "ps", "--filter", "name=^/"+name+"$", "--filter", "status=running", "--format", "{{.Names}}").Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == name
}

func (s *Service) runKafkaTopicsList(bootstrap string) ([]string, error) {
	bootstrap = strings.TrimSpace(bootstrap)
	if bootstrap == "" {
		bootstrap = "localhost:9092"
	}
	out, err := s.kafkaTopicsCommand(bootstrap, "--list")
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(out), "\n")
	topics := make([]string, 0)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			topics = append(topics, line)
		}
	}
	return topics, nil
}

func (s *Service) createTopic(cfg *KafkaAccelConfig, topic string) error {
	if cfg == nil {
		return fmt.Errorf("config required")
	}
	bootstrap := strings.TrimSpace(cfg.BootstrapServers)
	if bootstrap == "" {
		bootstrap = "localhost:9092"
	}
	partitions := cfg.TopicPartitions
	if partitions <= 0 {
		partitions = 3
	}
	replication := cfg.ReplicationFactor
	if replication <= 0 {
		replication = 1
	}
	retentionHours := cfg.RetentionHours
	if retentionHours <= 0 {
		retentionHours = 24
	}
	retentionMs := fmt.Sprintf("%d", retentionHours*3600*1000)
	_, err := s.kafkaTopicsCommand(
		bootstrap,
		"--create", "--if-not-exists",
		"--topic", topic,
		"--partitions", fmt.Sprintf("%d", partitions),
		"--replication-factor", fmt.Sprintf("%d", replication),
		"--config", "retention.ms="+retentionMs,
	)
	return err
}

func (s *Service) kafkaTopicsCommand(bootstrap string, args ...string) ([]byte, error) {
	scripts := []string{
		"/opt/kafka/bin/kafka-topics.sh",
		"/opt/bitnami/kafka/bin/kafka-topics.sh",
	}
	var lastErr error
	for _, script := range scripts {
		cmdArgs := append([]string{script, "--bootstrap-server", bootstrap}, args...)
		out, err := s.execKafka(cmdArgs...)
		if err == nil {
			return out, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("kafka-topics unavailable")
}

func (s *Service) execKafka(args ...string) ([]byte, error) {
	if !s.containerRunning(defaultContainer) {
		return nil, fmt.Errorf("container %s is not running", defaultContainer)
	}
	cmdArgs := append([]string{"exec", defaultContainer}, args...)
	return exec.Command("docker", cmdArgs...).CombinedOutput()
}
