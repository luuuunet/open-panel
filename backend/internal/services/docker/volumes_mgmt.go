package docker

import (
	"encoding/json"
	"fmt"
	"strings"
)

const panelVolumePrefix = "owpanel-"

func (s *Service) enrichVolumeUsage(list []Volume) []Volume {
	usage := s.buildVolumeUsageMap()
	for i := range list {
		v := &list[i]
		if names, ok := usage[v.Name]; ok {
			v.Containers = names
		} else {
			v.Containers = []string{}
		}
		v.InUse = len(v.Containers) > 0
		v.Category = classifyVolumeCategory(v.Name)
	}
	return list
}

func classifyVolumeCategory(name string) string {
	if strings.HasPrefix(name, panelVolumePrefix) {
		return "panel"
	}
	if isAnonymousVolumeName(name) {
		return "anonymous"
	}
	return "custom"
}

func isAnonymousVolumeName(name string) bool {
	if len(name) < 32 {
		return false
	}
	for _, c := range name {
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
			continue
		}
		return false
	}
	return true
}

func (s *Service) buildVolumeUsageMap() map[string][]string {
	result := make(map[string][]string)
	out, err := runDocker("ps", "-aq")
	if err != nil {
		return result
	}
	ids := strings.Fields(strings.TrimSpace(string(out)))
	if len(ids) == 0 {
		return result
	}
	inspectOut, err := runDocker(append([]string{"inspect"}, ids...)...)
	if err != nil {
		return result
	}
	var rows []struct {
		Name   string `json:"Name"`
		Mounts []struct {
			Type string `json:"Type"`
			Name string `json:"Name"`
		} `json:"Mounts"`
	}
	if json.Unmarshal(inspectOut, &rows) != nil {
		return result
	}
	for _, row := range rows {
		cname := strings.TrimPrefix(row.Name, "/")
		for _, m := range row.Mounts {
			if m.Type != "volume" || m.Name == "" {
				continue
			}
			seen := false
			for _, existing := range result[m.Name] {
				if existing == cname {
					seen = true
					break
				}
			}
			if !seen {
				result[m.Name] = append(result[m.Name], cname)
			}
		}
	}
	return result
}

func (s *Service) CreateVolume(name, driver string) error {
	if err := s.dockerOK(); err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("volume name required")
	}
	args := []string{"volume", "create", "--name", name}
	if d := strings.TrimSpace(driver); d != "" {
		args = append(args, "--driver", d)
	}
	_, err := runDocker(args...)
	return err
}

func (s *Service) RemoveVolume(name string, force bool) error {
	if err := s.dockerOK(); err != nil {
		return err
	}
	args := []string{"volume", "rm"}
	if force {
		args = append(args, "-f")
	}
	args = append(args, name)
	_, err := runDocker(args...)
	return err
}

func (s *Service) PruneVolumes() (string, error) {
	if err := s.dockerOK(); err != nil {
		return "", err
	}
	out, err := runDocker("volume", "prune", "-f")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
