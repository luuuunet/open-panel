package appstore

// dockerAppSpec defines a one-click Docker deployment.
type dockerAppSpec struct {
	Container string
	Image     string
	Port      string // host:container
	Env       []string
	Volumes   []string // host:container
}

// dockerAppSpecs — 内置 Docker 一键部署规格（需先安装 Docker）
var dockerAppSpecs = map[string]dockerAppSpec{
	// AI (shared with installer_ai)
	"open-webui":  {Container: "owpanel-open-webui", Image: "ghcr.io/open-webui/open-webui:main", Port: "8080:8080", Env: []string{"OLLAMA_BASE_URL=http://host.docker.internal:11434"}},
	"localai":     {Container: "owpanel-localai", Image: "localai/localai:latest", Port: "8090:8080"},
	"dify":        {Container: "owpanel-dify", Image: "langgenius/dify-web:latest", Port: "8091:3000"},
	"anythingllm": {Container: "owpanel-anythingllm", Image: "mintplexlabs/anythingllm:latest", Port: "3001:3001"},
	"fastgpt":     {Container: "owpanel-fastgpt", Image: "ghcr.io/labring/fastgpt:latest", Port: "3002:3000"},
	"comfyui":     {Container: "owpanel-comfyui", Image: "yanwk/comfyui-boot:cu124-slim", Port: "8188:8188"},
	"sd-webui":    {Container: "owpanel-sd-webui", Image: "continuumio/miniconda3:latest", Port: "7860:7860"},

	// 建站 / CMS
	"halo":              {Container: "owpanel-halo", Image: "halohub/halo:2.20", Port: "8090:8090"},
	"typecho":           {Container: "owpanel-typecho", Image: "joyqi/typecho:nightly-php8.2-apache", Port: "8080:80"},
	"wordpress-app":     {Container: "owpanel-wordpress", Image: "wordpress:6.7-php8.2-apache", Port: "8081:80", Env: []string{"WORDPRESS_DB_HOST=host.docker.internal"}},
	"ghost":             {Container: "owpanel-ghost", Image: "ghost:5-alpine", Port: "2368:2368", Env: []string{"url=http://localhost:2368"}},
	"outline":           {Container: "owpanel-outline", Image: "outlinewiki/outline:latest", Port: "3005:3000"},
	"memos":             {Container: "owpanel-memos", Image: "neosmemo/memos:stable", Port: "5230:5230"},
	"lsky-pro":          {Container: "owpanel-lsky", Image: "ddsderek/lsky-pro:latest", Port: "8099:80"},
	"flarum":            {Container: "owpanel-flarum", Image: "monologg/flarum-docker:latest", Port: "8889:8888"},

	// DevOps
	"portainer":    {Container: "owpanel-portainer", Image: "portainer/portainer-ce:latest", Port: "9000:9000", Volumes: []string{"/var/run/docker.sock:/var/run/docker.sock", "owpanel-portainer-data:/data"}},
	"gitea":        {Container: "owpanel-gitea", Image: "gitea/gitea:latest", Port: "3000:3000", Volumes: []string{"owpanel-gitea-data:/data"}},
	"jenkins":      {Container: "owpanel-jenkins", Image: "jenkins/jenkins:lts-jdk17", Port: "8082:8080", Volumes: []string{"owpanel-jenkins:/var/jenkins_home"}},
	"gitlab":       {Container: "owpanel-gitlab", Image: "gitlab/gitlab-ce:latest", Port: "8929:8929", Volumes: []string{"owpanel-gitlab-config:/etc/gitlab", "owpanel-gitlab-logs:/var/log/gitlab", "owpanel-gitlab-data:/var/opt/gitlab"}},
	"uptime-kuma":  {Container: "owpanel-uptime-kuma", Image: "louislam/uptime-kuma:1", Port: "3004:3001", Volumes: []string{"owpanel-uptime-kuma:/app/data"}},
	"netdata":      {Container: "owpanel-netdata", Image: "netdata/netdata:stable", Port: "19999:19999", Volumes: []string{"/proc:/host/proc:ro", "/sys:/host/sys:ro", "/var/run/docker.sock:/var/run/docker.sock:ro"}},
	"sonarqube":    {Container: "owpanel-sonarqube", Image: "sonarqube:lts-community", Port: "9002:9000", Volumes: []string{"owpanel-sonarqube-data:/opt/sonarqube/data"}},
	"frps":         {Container: "owpanel-frps", Image: "snowdreamtech/frps:latest", Port: "7500:7500", Volumes: []string{"owpanel-frps:/etc/frp"}},

	// 中间件
	"minio":           {Container: "owpanel-minio", Image: "minio/minio:latest", Port: "9001:9001", Env: []string{"MINIO_ROOT_USER=admin", "MINIO_ROOT_PASSWORD=openpanel123"}, Volumes: []string{"owpanel-minio:/data"}},
	"rabbitmq":        {Container: "owpanel-rabbitmq", Image: "rabbitmq:3-management-alpine", Port: "15672:15672", Env: []string{"RABBITMQ_DEFAULT_USER=admin", "RABBITMQ_DEFAULT_PASS=openpanel123"}},
	"elasticsearch":   {Container: "owpanel-elasticsearch", Image: "elasticsearch:8.11.0", Port: "9200:9200", Env: []string{"discovery.type=single-node", "xpack.security.enabled=false", "ES_JAVA_OPTS=-Xms512m -Xmx512m"}, Volumes: []string{"owpanel-es-data:/usr/share/elasticsearch/data"}},
	"nacos":           {Container: "owpanel-nacos", Image: "nacos/nacos-server:v2.3.2", Port: "8848:8848", Env: []string{"MODE=standalone"}},
	"etcd":            {Container: "owpanel-etcd", Image: "quay.io/coreos/etcd:v3.5.16", Port: "2379:2379", Env: []string{"ETCD_ADVERTISE_CLIENT_URLS=http://127.0.0.1:2379", "ETCD_LISTEN_CLIENT_URLS=http://0.0.0.0:2379"}},
	"traefik":         {Container: "owpanel-traefik", Image: "traefik:v3.0", Port: "8083:8080", Volumes: []string{"/var/run/docker.sock:/var/run/docker.sock:ro"}},
	"caddy":           {Container: "owpanel-caddy", Image: "caddy:2-alpine", Port: "2015:2015", Volumes: []string{"owpanel-caddy-data:/data", "owpanel-caddy-config:/config"}},

	// 数据库工具
	"mongo-express": {Container: "owpanel-mongo-express", Image: "mongo-express:latest", Port: "8082:8081", Env: []string{"ME_CONFIG_MONGODB_URL=mongodb://host.docker.internal:27017"}},
	"redis-insight": {Container: "owpanel-redis-insight", Image: "redis/redisinsight:latest", Port: "5540:5540"},
	"pgadmin":       {Container: "owpanel-pgadmin", Image: "dpage/pgadmin4:latest", Port: "5050:80", Env: []string{"PGADMIN_DEFAULT_EMAIL=admin@openpanel.local", "PGADMIN_DEFAULT_PASSWORD=openpanel123"}},

	// BI
	"kibana":    {Container: "owpanel-kibana", Image: "kibana:8.11.0", Port: "5601:5601", Env: []string{"ELASTICSEARCH_HOSTS=http://host.docker.internal:9200"}},
	"prometheus": {Container: "owpanel-prometheus", Image: "prom/prometheus:latest", Port: "9090:9090", Volumes: []string{"owpanel-prometheus:/prometheus"}},
	"grafana":   {Container: "owpanel-grafana", Image: "grafana/grafana:latest", Port: "3003:3000", Volumes: []string{"owpanel-grafana:/var/lib/grafana"}},
	"metabase":  {Container: "owpanel-metabase", Image: "metabase/metabase:latest", Port: "3007:3000"},
	"superset":  {Container: "owpanel-superset", Image: "apache/superset:latest", Port: "8089:8088"},

	// 开发工具
	"code-server": {Container: "owpanel-code-server", Image: "codercom/code-server:latest", Port: "8088:8080", Env: []string{"PASSWORD=openpanel123"}, Volumes: []string{"owpanel-code-server:/home/coder"}},
	"hoppscotch":  {Container: "owpanel-hoppscotch", Image: "hoppscotch/hoppscotch:latest", Port: "3006:3000"},

	// 多媒体 / 生活
	"home-assistant": {Container: "owpanel-homeassistant", Image: "ghcr.io/home-assistant/home-assistant:stable", Port: "8123:8123", Volumes: []string{"owpanel-ha:/config"}},
	"qbittorrent":    {Container: "owpanel-qbittorrent", Image: "lscr.io/linuxserver/qbittorrent:latest", Port: "8085:8080", Volumes: []string{"owpanel-qbit-config:/config", "owpanel-qbit-downloads:/downloads"}},
	"jellyfin":       {Container: "owpanel-jellyfin", Image: "jellyfin/jellyfin:latest", Port: "8096:8096", Volumes: []string{"owpanel-jellyfin-config:/config", "owpanel-jellyfin-cache:/cache"}},
	"navidrome":      {Container: "owpanel-navidrome", Image: "deluan/navidrome:latest", Port: "4533:4533", Volumes: []string{"owpanel-navidrome:/data"}},
	"emby":           {Container: "owpanel-emby", Image: "emby/embyserver:latest", Port: "8097:8096", Volumes: []string{"owpanel-emby:/config"}},
	"nextcloud":      {Container: "owpanel-nextcloud", Image: "nextcloud:latest", Port: "8087:80", Volumes: []string{"owpanel-nextcloud:/var/www/html"}},
	"syncthing":      {Container: "owpanel-syncthing", Image: "syncthing/syncthing:latest", Port: "8384:8384", Volumes: []string{"owpanel-syncthing:/var/syncthing"}},
	"alist":          {Container: "owpanel-alist", Image: "xhofe/alist:latest", Port: "5244:5244", Volumes: []string{"owpanel-alist:/opt/alist/data"}},
	"filebrowser":    {Container: "owpanel-filebrowser", Image: "filebrowser/filebrowser:latest", Port: "8086:80", Volumes: []string{"owpanel-filebrowser:/srv"}},

	// 安全
	"casdoor": {Container: "owpanel-casdoor", Image: "casbin/casdoor:latest", Port: "8000:8000"},

	// 图形处理
	"photoprism": {Container: "owpanel-photoprism", Image: "photoprism/photoprism:latest", Port: "2342:2342", Env: []string{"PHOTOPRISM_ADMIN_PASSWORD=openpanel123", "PHOTOPRISM_DATABASE_DRIVER=sqlite", "PHOTOPRISM_ORIGINALS_LIMIT=5000", "PHOTOPRISM_HTTP_COMPRESSION=gzip"}, Volumes: []string{"owpanel-photoprism-storage:/photoprism/storage", "owpanel-photoprism-originals:/photoprism/originals"}},
	"pigallery2": {Container: "owpanel-pigallery2", Image: "bpatrik/pigallery2:latest", Port: "3031:80", Volumes: []string{"owpanel-pigallery2:/app/data/config", "owpanel-pigallery2-db:/app/data/db", "owpanel-pigallery2-images:/app/data/images", "owpanel-pigallery2-tmp:/app/data/tmp"}},
	"imgproxy":   {Container: "owpanel-imgproxy", Image: "imgproxy/imgproxy:latest", Port: "3032:8080", Env: []string{"IMGPROXY_BIND=:8080", "IMGPROXY_USE_ETAG=true"}},
	"imagor":     {Container: "owpanel-imagor", Image: "shumc/imagor:latest", Port: "3033:8000", Env: []string{"IMAGOR_SECRET=openpanel123", "IMAGOR_AUTO_WEBP=true"}},
	"thumbor":    {Container: "owpanel-thumbor", Image: "beamerblbo/thumbor:latest", Port: "3037:8000", Env: []string{"THUMBOR_SECURITY_KEY=openpanel123"}},

	// 视频处理
	"tdarr":      {Container: "owpanel-tdarr", Image: "ghcr.io/haveagitgat/tdarr:latest", Port: "8265:8265", Env: []string{"serverIP=0.0.0.0", "serverPort=8266", "webUIPort=8265"}, Volumes: []string{"owpanel-tdarr-server:/app/server", "owpanel-tdarr-configs:/app/configs", "owpanel-tdarr-logs:/app/logs"}},
	"unmanic":    {Container: "owpanel-unmanic", Image: "ghcr.io/unmanic/unmanic:latest", Port: "3038:37488", Volumes: []string{"owpanel-unmanic-config:/config", "owpanel-unmanic-temp:/tmp/unmanic"}},
	"handbrake":  {Container: "owpanel-handbrake", Image: "lscr.io/linuxserver/handbrake:latest", Port: "5800:5800", Volumes: []string{"owpanel-handbrake-config:/config"}},
	"fileflows":  {Container: "owpanel-fileflows", Image: "revenz/fileflows:latest", Port: "3040:5000", Volumes: []string{"owpanel-fileflows:/app/Data"}},
}

func dockerSpec(key string) (dockerAppSpec, bool) {
	spec, ok := dockerAppSpecs[key]
	return spec, ok
}
