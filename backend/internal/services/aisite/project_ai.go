package aisite

import (
	"fmt"

	"github.com/luuuunet/owpanel/internal/services/aichat"
)

type SiteProjectChatRequest struct {
	Message   string           `json:"message"`
	Images    []string         `json:"images,omitempty"`
	FocusPath string           `json:"focus_path,omitempty"`
	Scope     string           `json:"scope"` // file | project
	History   []aichat.Message `json:"history"`
}

type SiteProjectApplyRequest struct {
	FileWrites []aichat.FileWriteSpec `json:"file_writes"`
}

type SiteProjectApplyResult struct {
	FilesWritten  []string `json:"files_written"`
	FailedFiles   []string `json:"failed_files,omitempty"`
	Logs          []string `json:"logs"`
}

func (s *Service) SiteProjectChat(siteID uint, req SiteProjectChatRequest) (*aichat.SiteProjectChatResult, error) {
	if err := s.aichat.EnsureConfigured(); err != nil {
		return nil, err
	}
	site, err := s.website.Get(siteID)
	if err != nil {
		return nil, err
	}
	scope := req.Scope
	if scope != "file" {
		scope = "project"
	}
	snap, err := s.website.BuildProjectSnapshotForChat(siteID, req.FocusPath, scope, req.Message)
	if err != nil {
		return nil, fmt.Errorf("读取项目文件失败: %w", err)
	}
	return s.aichat.SiteProjectChat(aichat.SiteProjectChatRequest{
		Message:   req.Message,
		Images:    req.Images,
		Domain:    site.Domain,
		RootPath:  site.RootPath,
		FocusPath: req.FocusPath,
		Scope:     scope,
		History:   req.History,
		Snapshot:  snap,
	})
}

func (s *Service) SiteProjectApply(siteID uint, req SiteProjectApplyRequest) (*SiteProjectApplyResult, error) {
	res := &SiteProjectApplyResult{Logs: []string{}}
	log := func(msg string) {
		res.Logs = append(res.Logs, msg)
	}
	if len(req.FileWrites) == 0 {
		return nil, fmt.Errorf("没有可应用的文件修改")
	}
	for _, fw := range req.FileWrites {
		log(fmt.Sprintf("写入 %s …", fw.RelativePath))
		if err := s.website.WriteSiteFile(siteID, fw.RelativePath, fw.Content); err != nil {
			log("  ✗ " + err.Error())
			res.FailedFiles = append(res.FailedFiles, fw.RelativePath)
			continue
		}
		log("  ✓ 完成")
		res.FilesWritten = append(res.FilesWritten, fw.RelativePath)
	}
	if len(res.FilesWritten) == 0 {
		return res, fmt.Errorf("未能写入任何文件")
	}
	return res, nil
}
