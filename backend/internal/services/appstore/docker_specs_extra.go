package appstore

func init() {
	for k, v := range dockerAppSpecsExtra {
		dockerAppSpecs[k] = v
	}
	// 兼容旧 key
	if spec, ok := dockerAppSpecs["wordpress-app"]; ok {
		dockerAppSpecs["wordpress"] = spec
	}
}

// dockerAppSpecsExtra — 内置扩展 Docker 部署规格（与 catalogExtraApps 对应）
var dockerAppSpecsExtra = map[string]dockerAppSpec{
	// AI
	"hermes-agent": {Container: "owpanel-hermes", Image: "nousresearch/hermes-agent:latest", Port: "8010:8010"},
	"openclaw":     {Container: "owpanel-openclaw", Image: "openclaw/openclaw:latest", Port: "8011:8011"},
	"qwenpow":      {Container: "owpanel-qwenpow", Image: "qwenlm/qwen-agent:latest", Port: "8012:8012"},
	"maxkb":        {Container: "owpanel-maxkb", Image: "1panel/maxkb:latest", Port: "8080:8080", Volumes: []string{"owpanel-maxkb:/opt/maxkb"}},
	"clawswarm":    {Container: "owpanel-clawswarm", Image: "openclaw/clawswarm:latest", Port: "8013:8013"},
	"upage":        {Container: "owpanel-upage", Image: "ghcr.io/upage/upage:latest", Port: "8014:3000"},
	"dbhub":        {Container: "owpanel-dbhub", Image: "bytebase/dbhub:latest", Port: "8015:8080"},
	"lobe-chat":    {Container: "owpanel-lobe-chat", Image: "lobehub/lobe-chat:latest", Port: "3210:3210", Env: []string{"OPENAI_API_KEY=sk-"}},
	"one-api":      {Container: "owpanel-one-api", Image: "justsong/one-api:latest", Port: "3000:3000", Volumes: []string{"owpanel-one-api:/data"}},
	"new-api":      {Container: "owpanel-new-api", Image: "calciumion/new-api:latest", Port: "3008:3000", Volumes: []string{"owpanel-new-api:/data"}},
	"chatgpt-next-web": {Container: "owpanel-next-web", Image: "yidadaa/chatgpt-next-web:latest", Port: "3009:3000"},
	"milvus":       {Container: "owpanel-milvus", Image: "milvusdb/milvus:latest", Port: "19530:19530", Volumes: []string{"owpanel-milvus:/var/lib/milvus"}},
	"qdrant":       {Container: "owpanel-qdrant", Image: "qdrant/qdrant:latest", Port: "6333:6333", Volumes: []string{"owpanel-qdrant:/qdrant/storage"}},
	"flowise":      {Container: "owpanel-flowise", Image: "flowiseai/flowise:latest", Port: "3010:3000", Volumes: []string{"owpanel-flowise:/root/.flowise"}},

	// 网站 / DevOps / 安全
	"wordpress":            {Container: "owpanel-wordpress", Image: "wordpress:6.7-php8.2-apache", Port: "8081:80", Env: []string{"WORDPRESS_DB_HOST=host.docker.internal"}},
	"jumpserver":           {Container: "owpanel-jumpserver", Image: "jumpserver/jms_all:latest", Port: "8088:80", Volumes: []string{"owpanel-jms:/opt/jumpserver/data"}},
	"nginx-proxy-manager":  {Container: "owpanel-npm", Image: "jc21/nginx-proxy-manager:latest", Port: "81:81", Volumes: []string{"owpanel-npm-data:/data", "owpanel-npm-letsencrypt:/etc/letsencrypt"}},
	"tomcat":               {Container: "owpanel-tomcat", Image: "tomcat:10-jdk17", Port: "8080:8080"},
	"dashy":                {Container: "owpanel-dashy", Image: "lissy93/dashy:latest", Port: "8083:8080", Volumes: []string{"owpanel-dashy:/app/user-data"}},
	"discourse":            {Container: "owpanel-discourse", Image: "discourse/discourse:latest", Port: "8092:80"},
	"bytebase":             {Container: "owpanel-bytebase", Image: "bytebase/bytebase:latest", Port: "8085:8080", Volumes: []string{"owpanel-bytebase:/var/opt/bytebase"}},
	"nocobase":             {Container: "owpanel-nocobase", Image: "nocobase/nocobase:latest", Port: "8084:80", Volumes: []string{"owpanel-nocobase:/app/nocobase/storage"}},
	"verdaccio":            {Container: "owpanel-verdaccio", Image: "verdaccio/verdaccio:latest", Port: "4873:4873", Volumes: []string{"owpanel-verdaccio:/verdaccio/storage"}},
	"opengist":             {Container: "owpanel-opengist", Image: "ghcr.io/opengist/opengist:latest", Port: "8090:6157", Volumes: []string{"owpanel-opengist:/opengist"}},

	// 监控 / 工具
	"zabbix-server": {Container: "owpanel-zabbix", Image: "zabbix/zabbix-server-mysql:ubuntu-6.4-latest", Port: "10051:10051"},
	"zabbix-agent":  {Container: "owpanel-zabbix-agent", Image: "zabbix/zabbix-agent2:ubuntu-6.4-latest", Port: "10050:10050", Env: []string{"ZBX_HOSTNAME=owpanel"}},
	"sftpgo":        {Container: "owpanel-sftpgo", Image: "drakkan/sftpgo:latest", Port: "8082:8080", Volumes: []string{"owpanel-sftpgo:/srv/sftpgo"}},
	"synapse":       {Container: "owpanel-synapse", Image: "matrixdotorg/synapse:latest", Port: "8008:8008", Volumes: []string{"owpanel-synapse:/data"}},
	"dockge":        {Container: "owpanel-dockge", Image: "louislam/dockge:1", Port: "5001:5001", Volumes: []string{"/var/run/docker.sock:/var/run/docker.sock", "owpanel-dockge:/app/data", "owpanel-dockge-stacks:/opt/stacks"}},
	"meilisearch":   {Container: "owpanel-meilisearch", Image: "getmeili/meilisearch:latest", Port: "7700:7700", Volumes: []string{"owpanel-meilisearch:/meili_data"}, Env: []string{"MEILI_MASTER_KEY=openpanel123"}},
	"ntfy":          {Container: "owpanel-ntfy", Image: "binwiederhier/ntfy:latest", Port: "8086:80", Volumes: []string{"owpanel-ntfy:/var/lib/ntfy"}},
	"openvpn":       {Container: "owpanel-openvpn", Image: "kylemanna/openvpn:latest", Port: "1194:1194/udp", Volumes: []string{"owpanel-openvpn:/etc/openvpn"}},
	"watchtower":    {Container: "owpanel-watchtower", Image: "containrrr/watchtower:latest", Volumes: []string{"/var/run/docker.sock:/var/run/docker.sock"}},
	"homepage":      {Container: "owpanel-homepage", Image: "ghcr.io/gethomepage/homepage:latest", Port: "8089:3000", Volumes: []string{"owpanel-homepage:/app/config", "/var/run/docker.sock:/var/run/docker.sock:ro"}},
	"redpanda-console": {Container: "owpanel-redpanda-console", Image: "redpandadata/console:latest", Port: "8091:8080"},
	"obsidian-livesync": {Container: "owpanel-livesync", Image: "vrtmrz/obsidian-livesync-server:latest", Port: "8093:5984", Volumes: []string{"owpanel-livesync:/usr/local/var/lib/couchdb"}},
	"screego":       {Container: "owpanel-screego", Image: "screego/server:latest", Port: "8096:5050"},
	"cloudflared":   {Container: "owpanel-cloudflared", Image: "cloudflare/cloudflared:latest", Env: []string{"TUNNEL_TOKEN="}},
	"beszel":        {Container: "owpanel-beszel", Image: "henrygd/beszel:latest", Port: "8099:8090", Volumes: []string{"owpanel-beszel:/beszel_data"}},
	"wallos":        {Container: "owpanel-wallos", Image: "bellamy/wallos:latest", Port: "8100:80", Volumes: []string{"owpanel-wallos:/var/www/html"}},
	"teable":        {Container: "owpanel-teable", Image: "teableio/teable:latest", Port: "8102:3000", Volumes: []string{"owpanel-teable:/app/.assets"}},
	"rustfs":        {Container: "owpanel-rustfs", Image: "rustfs/rustfs:latest", Port: "8105:9000", Volumes: []string{"owpanel-rustfs:/data"}},
	"n8n":           {Container: "owpanel-n8n", Image: "n8nio/n8n:latest", Port: "5678:5678", Volumes: []string{"owpanel-n8n:/home/node/.n8n"}},
	"activepieces":  {Container: "owpanel-activepieces", Image: "activepieces/activepieces:latest", Port: "5680:80", Volumes: []string{"owpanel-ap:/root/.activepieces"}},
	"rustdesk":      {Container: "owpanel-rustdesk-hb", Image: "rustdesk/rustdesk-server:latest", Port: "21115:21115", Volumes: []string{"owpanel-rustdesk:/root"}},
	"adguard-home":  {Container: "owpanel-adguard", Image: "adguard/adguardhome:latest", Port: "3011:3000", Volumes: []string{"owpanel-adguard:/opt/adguardhome/work"}},
	"stirling-pdf":  {Container: "owpanel-stirling", Image: "frooodle/s-pdf:latest", Port: "3016:8080", Volumes: []string{"owpanel-stirling:/usr/share/tesseract-ocr"}},
	"it-tools":      {Container: "owpanel-it-tools", Image: "corentinth/it-tools:latest", Port: "3017:80"},
	"searxng":       {Container: "owpanel-searxng", Image: "searxng/searxng:latest", Port: "3020:8080", Volumes: []string{"owpanel-searxng:/etc/searxng"}},
	"drawio":        {Container: "owpanel-drawio", Image: "jgraph/drawio:latest", Port: "3029:8080"},
	"excalidraw":    {Container: "owpanel-excalidraw", Image: "excalidraw/excalidraw:latest", Port: "3030:80"},
	"mattermost":    {Container: "owpanel-mattermost", Image: "mattermost/mattermost-team-edition:latest", Port: "8065:8065", Volumes: []string{"owpanel-mattermost:/mattermost/data"}},
	"rocketchat":    {Container: "owpanel-rocketchat", Image: "rocket.chat:latest", Port: "3024:3000", Volumes: []string{"owpanel-rocketchat:/app/uploads"}},
	"adminer":       {Container: "owpanel-adminer", Image: "adminer:latest", Port: "3025:8080"},
	"affine":        {Container: "owpanel-affine", Image: "ghcr.io/toeverything/affine-graphql:latest", Port: "3027:3010", Volumes: []string{"owpanel-affine:/root/.affine"}},
	"trilium":       {Container: "owpanel-trilium", Image: "zadam/trilium:latest", Port: "3028:8080", Volumes: []string{"owpanel-trilium:/home/node/trilium-data"}},

	// 多媒体
	"komga":           {Container: "owpanel-komga", Image: "gotson/komga:latest", Port: "8087:25600", Volumes: []string{"owpanel-komga:/config"}},
	"audiobookshelf":  {Container: "owpanel-audiobookshelf", Image: "ghcr.io/advplyr/audiobookshelf:latest", Port: "8094:80", Volumes: []string{"owpanel-abs:/config", "owpanel-abs-meta:/metadata"}},
	"immich":          {Container: "owpanel-immich", Image: "ghcr.io/immich-app/immich-server:release", Port: "3013:2283", Volumes: []string{"owpanel-immich:/usr/src/app/upload"}},

	// 数据库 / 中间件
	"typesense":   {Container: "owpanel-typesense", Image: "typesense/typesense:26.0", Port: "8108:8108", Volumes: []string{"owpanel-typesense:/data"}, Env: []string{"TYPESENSE_API_KEY=openpanel123", "TYPESENSE_DATA_DIR=/data"}},
	"kafka":       {Container: "owpanel-kafka", Image: "apache/kafka:3.9.0", Port: "9092:9092", Env: []string{"KAFKA_NODE_ID=1", "KAFKA_PROCESS_ROLES=broker,controller", "KAFKA_LISTENERS=PLAINTEXT://:9092,CONTROLLER://:9093", "KAFKA_ADVERTISED_LISTENERS=PLAINTEXT://localhost:9092", "KAFKA_CONTROLLER_LISTENER_NAMES=CONTROLLER", "KAFKA_LISTENER_SECURITY_PROTOCOL_MAP=CONTROLLER:PLAINTEXT,PLAINTEXT:PLAINTEXT", "KAFKA_CONTROLLER_QUORUM_VOTERS=1@localhost:9093", "KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR=1", "KAFKA_TRANSACTION_STATE_LOG_REPLICATION_FACTOR=1", "KAFKA_TRANSACTION_STATE_LOG_MIN_ISR=1", "KAFKA_GROUP_INITIAL_REBALANCE_DELAY_MS=0", "KAFKA_LOG_RETENTION_HOURS=24", "KAFKA_LOG_RETENTION_CHECK_INTERVAL_MS=300000", "KAFKA_HEAP_OPTS=-Xmx512M -Xms256M"}},
	"cloudbeaver": {Container: "owpanel-cloudbeaver", Image: "dbeaver/cloudbeaver:latest", Port: "8088:8978", Volumes: []string{"owpanel-cloudbeaver:/opt/cloudbeaver/workspace"}},
	"jaeger":      {Container: "owpanel-jaeger", Image: "jaegertracing/all-in-one:latest", Port: "16686:16686"},
	"redis-commander": {Container: "owpanel-redis-commander", Image: "rediscommander/redis-commander:latest", Port: "8095:8081", Env: []string{"REDIS_HOSTS=local:host.docker.internal:6379"}},
	"influxdb":    {Container: "owpanel-influxdb", Image: "influxdb:2.7", Port: "8086:8086", Volumes: []string{"owpanel-influxdb:/var/lib/influxdb2"}},
	"clickhouse":  {Container: "owpanel-clickhouse", Image: "clickhouse/clickhouse-server:latest", Port: "8123:8123", Volumes: []string{"owpanel-clickhouse:/var/lib/clickhouse"}},

	// 邮件 / CRM / BI
	"stalwart":  {Container: "owpanel-stalwart", Image: "stalwartlabs/stalwart:latest", Port: "8098:8080", Volumes: []string{"owpanel-stalwart:/opt/stalwart-mail"}},
	"twenty":    {Container: "owpanel-twenty", Image: "twentyhq/twenty:latest", Port: "8103:3000", Volumes: []string{"owpanel-twenty:/app/packages/twenty-server/.local-storage"}},
	"odoo":      {Container: "owpanel-odoo", Image: "odoo:17", Port: "8104:8069", Volumes: []string{"owpanel-odoo:/var/lib/odoo"}, Env: []string{"HOST=host.docker.internal", "USER=odoo", "PASSWORD=odoo"}},
	"jupyter-notebook": {Container: "owpanel-jupyter-nb", Image: "jupyter/scipy-notebook:latest", Port: "8889:8888", Env: []string{"JUPYTER_ENABLE_LAB=yes"}, Volumes: []string{"owpanel-jupyter-nb:/home/jovyan/work"}},
	"umami":     {Container: "owpanel-umami", Image: "ghcr.io/umami-software/umami:postgresql-latest", Port: "3018:3000"},
	"matomo":    {Container: "owpanel-matomo", Image: "matomo:latest", Port: "3019:80", Volumes: []string{"owpanel-matomo:/var/www/html"}},
	"nocodb":    {Container: "owpanel-nocodb", Image: "nocodb/nocodb:latest", Port: "3021:8080", Volumes: []string{"owpanel-nocodb:/usr/app/data"}},

	// 安全
	"zitadel":      {Container: "owpanel-zitadel", Image: "ghcr.io/zitadel/zitadel:latest", Port: "8101:8080"},
	"vaultwarden":  {Container: "owpanel-vaultwarden", Image: "vaultwarden/server:latest", Port: "3012:80", Volumes: []string{"owpanel-vw:/data"}},
	"keycloak":     {Container: "owpanel-keycloak", Image: "quay.io/keycloak/keycloak:latest", Port: "3026:8080", Env: []string{"KEYCLOAK_ADMIN=admin", "KEYCLOAK_ADMIN_PASSWORD=openpanel123"}},

	// 云存储 / 文档
	"paperless-ngx": {Container: "owpanel-paperless", Image: "ghcr.io/paperless-ngx/paperless-ngx:latest", Port: "3014:8000", Volumes: []string{"owpanel-paperless:/usr/src/paperless/data"}},
	"firefly-iii":   {Container: "owpanel-firefly", Image: "fireflyiii/core:latest", Port: "3015:8080", Volumes: []string{"owpanel-firefly:/var/www/html/storage"}},
	"bookstack":     {Container: "owpanel-bookstack", Image: "lscr.io/linuxserver/bookstack:latest", Port: "3022:80", Volumes: []string{"owpanel-bookstack:/config"}},
	"wiki-js":       {Container: "owpanel-wikijs", Image: "requarks/wiki:2", Port: "3023:3000", Volumes: []string{"owpanel-wikijs:/wiki/data"}},

	// 实验性
	"windows": {Container: "owpanel-windows", Image: "dockurr/windows:latest", Port: "8006:8006", Volumes: []string{"owpanel-windows:/storage"}, Env: []string{"RAM_SIZE=4G", "CPU_CORES=2"}},
}
