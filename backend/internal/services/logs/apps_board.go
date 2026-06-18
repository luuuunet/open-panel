package logs

// InstalledAppBoard is a dashboard row/card for an installed store app and its log files.
type InstalledAppBoard struct {
	Key        string   `json:"key"`
	Name       string   `json:"name"`
	Icon       string   `json:"icon"`
	IconURL    string   `json:"icon_url"`
	Status     string   `json:"status"`
	LiveStatus string   `json:"live_status"`
	Version    string   `json:"version"`
	Port       int      `json:"port"`
	Category   string   `json:"category"`
	Logs       []Source `json:"logs"`
}

func liveRank(s string) int {
	switch s {
	case "running":
		return 0
	case "installing":
		return 1
	case "stopped":
		return 2
	case "failed":
		return 3
	default:
		return 4
	}
}

// LiveRank exposes running-state sort order for handlers.
func LiveRank(s string) int { return liveRank(s) }
