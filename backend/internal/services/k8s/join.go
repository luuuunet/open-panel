package k8s

import (
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/luuuunet/owpanel/internal/services/appstore"
)

const k3sNodeTokenPath = "/var/lib/rancher/k3s/server/node-token"

type JoinInfo struct {
	ServerURL string            `json:"server_url"`
	Token     string            `json:"token"`
	Commands  map[string]string `json:"commands"`
	Script    string            `json:"script"`
}

func (s *Service) JoinInfo() (*JoinInfo, error) {
	if !s.linuxHost() {
		return nil, fmt.Errorf("K8s 加入节点需在 Linux 服务器上运行")
	}
	if !s.k3sRunning() {
		return nil, fmt.Errorf("k3s 未运行，请先安装并启动 k3s")
	}

	host := s.serverHost()
	serverURL := fmt.Sprintf("https://%s:6443", host)
	token, err := readK3sToken()
	if err != nil {
		return nil, err
	}

	info := &JoinInfo{
		ServerURL: serverURL,
		Token:     token,
		Commands:  map[string]string{},
	}
	workerCmd := fmt.Sprintf(
		`curl -sfL https://get.k3s.io | K3S_URL=%s K3S_TOKEN=%s sh -`,
		serverURL, token,
	)
	info.Commands["worker"] = workerCmd
	info.Script = generateJoinScript(serverURL, token)
	return info, nil
}

func (s *Service) serverHost() string {
	if s.settings != nil {
		all, _ := s.settings.GetAll()
		if ip := strings.TrimSpace(all["server_public_ip"]); ip != "" {
			if h, _, err := net.SplitHostPort(ip); err == nil {
				return h
			}
			return ip
		}
	}
	out, err := kubectl("get", "nodes", "-o", "jsonpath={.items[0].status.addresses[?(@.type==\"InternalIP\")].address}")
	if err == nil {
		if ip := strings.TrimSpace(out); ip != "" {
			return ip
		}
	}
	return "127.0.0.1"
}

func readK3sToken() (string, error) {
	data, err := os.ReadFile(k3sNodeTokenPath)
	if err != nil {
		return "", fmt.Errorf("无法读取 k3s 节点 token（%s）: %w", k3sNodeTokenPath, err)
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", fmt.Errorf("k3s 节点 token 为空")
	}
	return token, nil
}

func generateJoinScript(serverURL, token string) string {
	return fmt.Sprintf(`#!/bin/bash
# OWPanel K3s worker join script
set -e
curl -sfL https://get.k3s.io | K3S_URL=%s K3S_TOKEN=%s sh -
echo "[owpanel] K3s agent joined successfully"
`, serverURL, token)
}

type InstallResult struct {
	Message string `json:"message"`
	K3s     bool   `json:"k3s"`
}

func (s *Service) Install() (*InstallResult, error) {
	if !s.linuxHost() {
		return nil, fmt.Errorf("K3s 仅支持 Linux 服务器")
	}
	res := &InstallResult{}
	if !s.k3sRunning() {
		if err := appstore.RunK3sInstall(s.dataDir); err != nil {
			return nil, fmt.Errorf("k3s 安装失败: %w", err)
		}
		s.markInstalled(k3sAppKey)
		res.K3s = true
	} else {
		res.K3s = true
	}
	res.Message = "K3s 安装完成"
	return res, nil
}
