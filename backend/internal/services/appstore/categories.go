package appstore

import (
	"sort"

	"github.com/luuuunet/owpanel/internal/models"
)

// StoreCategoryOrder is the preferred tab order in the software store.
var StoreCategoryOrder = []string{
	"Web服务器",
	"运行环境",
	"数据库",
	"中间件",
	"容器",
	"FTP",
	"邮件",
	"网站",
	"人工智能",
	"图形处理",
	"视频处理",
	"多媒体",
	"DevOps",
	"开发工具",
	"BI",
	"CRM",
	"安全",
	"云存储",
	"工具",
	"生活",
}

var categoryAliases = map[string]string{
	"邮件服务":       "邮件",
	"郵件服務":       "邮件",
	"郵件":         "邮件",
	"Email":        "邮件",
	"系统工具":       "工具",
	"系統工具":       "工具",
	"System Tools": "工具",
	"存储":         "云存储",
	"儲存":         "云存储",
	"雲儲存":         "云存储",
	"建站":         "网站",
	"網站":         "网站",
	"Web Server":   "Web服务器",
	"Web伺服器":      "Web服务器",
	"Database":     "数据库",
	"資料庫":         "数据库",
	"Runtime":      "运行环境",
	"執行環境":        "运行环境",
	"Container":    "容器",
	"中間件":         "中间件",
	"開發工具":        "开发工具",
	"多媒體":         "多媒体",
}

func NormalizeCategory(category string) string {
	if c, ok := categoryAliases[category]; ok {
		return c
	}
	return category
}

func categoryRank() map[string]int {
	rank := make(map[string]int, len(StoreCategoryOrder))
	for i, c := range StoreCategoryOrder {
		rank[c] = i
	}
	return rank
}

func SortAppsByCategory(apps []models.App) {
	rank := categoryRank()
	sort.Slice(apps, func(i, j int) bool {
		ci := rank[NormalizeCategory(apps[i].Category)]
		cj := rank[NormalizeCategory(apps[j].Category)]
		if ci != cj {
			return ci < cj
		}
		if apps[i].Category != apps[j].Category {
			return apps[i].Category < apps[j].Category
		}
		return apps[i].Name < apps[j].Name
	})
}

func (s *Service) normalizeStoredCategories() {
	var apps []models.App
	if err := s.db.Find(&apps).Error; err != nil {
		return
	}
	for _, app := range apps {
		norm := NormalizeCategory(app.Category)
		if norm == app.Category {
			continue
		}
		_ = s.db.Model(&app).Update("category", norm).Error
	}
}
