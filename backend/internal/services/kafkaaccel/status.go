package kafkaaccel

import (
	"net"
	"strings"
	"time"
)

type StatusResult struct {
	KafkaInstalled  bool   `json:"kafka_installed"`
	KafkaRunning    bool   `json:"kafka_running"`
	BrokerReachable bool   `json:"broker_reachable"`
	ContainerName   string `json:"container_name"`
	Bootstrap       string `json:"bootstrap_servers"`
	Hint            string `json:"hint,omitempty"`
}

func (s *Service) kafkaInstalled() bool {
	if s.apps == nil {
		return false
	}
	app, err := s.apps.Get(kafkaAppKey)
	if err == nil && app.Installed {
		return true
	}
	return s.containerRunning(defaultContainer)
}

func (s *Service) kafkaRunning() bool {
	if s.apps != nil {
		if st := s.apps.LiveStatus(kafkaAppKey); st == "running" {
			return true
		}
	}
	return s.containerRunning(defaultContainer)
}

func (s *Service) Status() (*StatusResult, error) {
	cfg, err := s.GetConfig()
	if err != nil {
		return nil, err
	}
	bootstrap := strings.TrimSpace(cfg.BootstrapServers)
	if bootstrap == "" {
		bootstrap = "127.0.0.1:9092"
	}
	installed := s.kafkaInstalled()
	running := s.kafkaRunning()
	reachable := s.brokerReachable(bootstrap)

	out := &StatusResult{
		KafkaInstalled:  installed,
		KafkaRunning:    running,
		BrokerReachable: reachable,
		ContainerName:   defaultContainer,
		Bootstrap:       bootstrap,
	}
	if !installed {
		out.Hint = "Install Kafka from the App Store (Docker app kafka, port 9092)"
	} else if !running {
		out.Hint = "Start the Kafka service or Docker container owpanel-kafka"
	} else if !reachable {
		out.Hint = "Kafka is running but broker is not reachable at " + bootstrap
	}
	return out, nil
}

func (s *Service) brokerReachable(bootstrap string) bool {
	host, port, ok := splitHostPort(bootstrap, "9092")
	if !ok {
		return false
	}
	addr := net.JoinHostPort(host, port)
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func splitHostPort(raw, defaultPort string) (string, string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "127.0.0.1", defaultPort, true
	}
	if strings.Contains(raw, ",") {
		raw = strings.TrimSpace(strings.Split(raw, ",")[0])
	}
	if h, p, err := net.SplitHostPort(raw); err == nil {
		if h == "" {
			h = "127.0.0.1"
		}
		return h, p, true
	}
	return raw, defaultPort, true
}
