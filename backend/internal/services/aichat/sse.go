package aichat

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

type streamEvent struct {
	Content string `json:"content,omitempty"`
	Done    bool   `json:"done,omitempty"`
	Error   string `json:"error,omitempty"`
}

func writeSSE(c *gin.Context, ev streamEvent) {
	data, _ := json.Marshal(ev)
	fmt.Fprintf(c.Writer, "data: %s\n\n", data)
	if f, ok := c.Writer.(http.Flusher); ok {
		f.Flush()
	}
}

// StreamError writes an SSE error event and closes the stream.
func StreamError(c *gin.Context, msg string) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Status(http.StatusOK)
	writeSSE(c, streamEvent{Error: msg, Done: true})
}

func (s *Service) StreamSiteLogChat(req SiteLogChatRequest, c *gin.Context) {
	if err := s.EnsureConfigured(); err != nil {
		writeSSE(c, streamEvent{Error: err.Error(), Done: true})
		return
	}

	access := trimLogForAI(req.AccessLog, 40000)
	errLog := trimLogForAI(req.ErrorLog, 40000)
	system := fmt.Sprintf(`你是 OWPanel 网站日志与运维 AI 助手（类似 Cursor IDE 助手）。
站点域名: %s
网站根目录: %s
访问日志: %s
错误日志: %s

要求:
- 用 Markdown 格式回答（可含代码块、列表）
- 分析访问/错误日志中的异常、攻击、404、PHP/Nginx 错误
- 给出可操作的排查与修复步骤；若需创建文件，给出完整路径与内容
- 不要编造日志中不存在的内容

访问日志:
%s

错误日志:
%s`, req.Domain, req.RootPath, req.AccessPath, req.ErrorPath, access, errLog)

	messages := []Message{{Role: "system", Content: system}}
	for _, h := range req.History {
		if h.Role == "user" || h.Role == "assistant" {
			messages = append(messages, h)
		}
	}
	messages = append(messages, Message{Role: "user", Content: req.Message})

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Status(http.StatusOK)

	streamErr := s.ChatStream(messages, func(chunk string) error {
		writeSSE(c, streamEvent{Content: chunk})
		return nil
	})
	if streamErr != nil {
		writeSSE(c, streamEvent{Error: streamErr.Error()})
	}
	writeSSE(c, streamEvent{Done: true})
}

func (s *Service) StreamLogChat(req LogChatRequest, c *gin.Context) {
	if err := s.EnsureConfigured(); err != nil {
		writeSSE(c, streamEvent{Error: err.Error(), Done: true})
		return
	}
	name := req.SourceName
	if name == "" {
		name = req.SourceID
	}
	logText := trimLogForAI(req.LogContent, 80000)
	system := fmt.Sprintf(`你是 OWPanel 日志分析助手（类似 Cursor IDE）。
来源: %s | 分类: %s | 路径: %s

用 Markdown 回答，指出错误模式与修复建议。日志:
%s`, name, req.Category, req.Path, logText)

	messages := []Message{{Role: "system", Content: system}}
	for _, h := range req.History {
		if h.Role == "user" || h.Role == "assistant" {
			messages = append(messages, h)
		}
	}
	messages = append(messages, Message{Role: "user", Content: req.Message})

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Status(http.StatusOK)

	if err := s.ChatStream(messages, func(chunk string) error {
		writeSSE(c, streamEvent{Content: chunk})
		return nil
	}); err != nil {
		writeSSE(c, streamEvent{Error: err.Error()})
	}
	writeSSE(c, streamEvent{Done: true})
}
