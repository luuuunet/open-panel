package api

import (
	"github.com/gin-gonic/gin"
	"github.com/luuuunet/owpanel/internal/api/response"
	"github.com/luuuunet/owpanel/internal/services/kafkaaccel"
)

func (s *Server) registerKafkaAccelRoutes(admin *gin.RouterGroup) {
	admin.GET("/kafka-accel/config", s.handleGetKafkaAccelConfig)
	admin.PATCH("/kafka-accel/config", s.handlePatchKafkaAccelConfig)
	admin.GET("/kafka-accel/status", s.handleKafkaAccelStatus)
	admin.GET("/kafka-accel/topics", s.handleKafkaAccelTopics)
	admin.POST("/kafka-accel/apply", s.handleKafkaAccelApply)
	admin.POST("/kafka-accel/presets/:key", s.handleKafkaAccelPreset)
	admin.POST("/kafka-accel/auto-enable", s.handleKafkaAccelAutoEnable)
	admin.POST("/kafka-accel/databases/:id/auto-enable", s.handleKafkaAccelAutoEnableDatabase)
}

func (s *Server) handleGetKafkaAccelConfig(c *gin.Context) {
	cfg, err := s.kafkaaccel.GetConfig()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{
		"config":              cfg,
		"linked_database_ids": kafkaaccel.ParseLinkedIDs(cfg.LinkedDatabaseIDs),
		"expected_topics":     s.kafkaaccel.ExpectedTopics(cfg),
	})
}

func (s *Server) handlePatchKafkaAccelConfig(c *gin.Context) {
	var req struct {
		Enabled           *bool  `json:"enabled"`
		BootstrapServers  string `json:"bootstrap_servers"`
		TopicPrefix       string `json:"topic_prefix"`
		Mode              string `json:"mode"`
		LinkedDatabaseIDs []uint `json:"linked_database_ids"`
		ConsumerGroup     string `json:"consumer_group"`
		TopicPartitions   *int   `json:"topic_partitions"`
		ReplicationFactor *int   `json:"replication_factor"`
		RetentionHours    *int   `json:"retention_hours"`
		ProducerBatchSize *int   `json:"producer_batch_size"`
		ProducerLingerMs  *int   `json:"producer_linger_ms"`
		CompressionType   string `json:"compression_type"`
		FetchMinBytes     *int   `json:"fetch_min_bytes"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, err.Error())
		return
	}
	cfg, err := s.kafkaaccel.GetConfig()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	patch := *cfg
	if req.Enabled != nil {
		patch.Enabled = *req.Enabled
	}
	if req.BootstrapServers != "" {
		patch.BootstrapServers = req.BootstrapServers
	}
	if req.TopicPrefix != "" {
		patch.TopicPrefix = req.TopicPrefix
	}
	if req.Mode != "" {
		patch.Mode = req.Mode
	}
	if req.ConsumerGroup != "" {
		patch.ConsumerGroup = req.ConsumerGroup
	}
	if req.LinkedDatabaseIDs != nil {
		patch.LinkedDatabaseIDs = kafkaaccel.FormatLinkedIDs(req.LinkedDatabaseIDs)
	}
	if req.TopicPartitions != nil {
		patch.TopicPartitions = *req.TopicPartitions
	}
	if req.ReplicationFactor != nil {
		patch.ReplicationFactor = *req.ReplicationFactor
	}
	if req.RetentionHours != nil {
		patch.RetentionHours = *req.RetentionHours
	}
	if req.ProducerBatchSize != nil {
		patch.ProducerBatchSize = *req.ProducerBatchSize
	}
	if req.ProducerLingerMs != nil {
		patch.ProducerLingerMs = *req.ProducerLingerMs
	}
	if req.CompressionType != "" {
		patch.CompressionType = req.CompressionType
	}
	if req.FetchMinBytes != nil {
		patch.FetchMinBytes = *req.FetchMinBytes
	}
	updated, err := s.kafkaaccel.UpdateConfig(&patch)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, gin.H{
		"config":              updated,
		"linked_database_ids": kafkaaccel.ParseLinkedIDs(updated.LinkedDatabaseIDs),
		"expected_topics":     s.kafkaaccel.ExpectedTopics(updated),
	})
}

func (s *Server) handleKafkaAccelPreset(c *gin.Context) {
	res, err := s.kafkaaccel.ApplyPreset(c.Param("key"))
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, res)
}

func (s *Server) handleKafkaAccelStatus(c *gin.Context) {
	st, err := s.kafkaaccel.Status()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, st)
}

func (s *Server) handleKafkaAccelTopics(c *gin.Context) {
	res, err := s.kafkaaccel.ListTopics()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, res)
}

func (s *Server) handleKafkaAccelApply(c *gin.Context) {
	res, err := s.kafkaaccel.Apply()
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, res)
}

func (s *Server) handleKafkaAccelAutoEnable(c *gin.Context) {
	var req struct {
		InstallKafka bool `json:"install_kafka"`
	}
	_ = c.ShouldBindJSON(&req)
	install := true
	if c.Request.ContentLength > 0 {
		install = req.InstallKafka
	}
	res, err := s.kafkaaccel.AutoEnable(install)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, res)
}

func (s *Server) handleKafkaAccelAutoEnableDatabase(c *gin.Context) {
	var req struct {
		InstallKafka bool `json:"install_kafka"`
	}
	_ = c.ShouldBindJSON(&req)
	install := true
	if c.Request.ContentLength > 0 {
		install = req.InstallKafka
	}
	res, err := s.kafkaaccel.AutoEnableForDatabase(parseID(c), install)
	if err != nil {
		response.Error(c, 500, err.Error())
		return
	}
	response.OK(c, res)
}
