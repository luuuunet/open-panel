package compose

import "fmt"

type Template struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

var composeTemplates = map[string]string{
	"nginx": `services:
  web:
    image: nginx:alpine
    ports:
      - "8080:80"
    restart: unless-stopped
`,
	"mysql": `services:
  db:
    image: mysql:8
    environment:
      MYSQL_ROOT_PASSWORD: changeme
      MYSQL_DATABASE: app
    ports:
      - "3306:3306"
    volumes:
      - mysql_data:/var/lib/mysql
    restart: unless-stopped
volumes:
  mysql_data:
`,
	"redis": `services:
  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    restart: unless-stopped
`,
	"wordpress": `services:
  db:
    image: mysql:8
    environment:
      MYSQL_ROOT_PASSWORD: changeme
      MYSQL_DATABASE: wordpress
    volumes:
      - db_data:/var/lib/mysql
    restart: unless-stopped
  wordpress:
    image: wordpress:latest
    ports:
      - "8080:80"
    environment:
      WORDPRESS_DB_HOST: db
      WORDPRESS_DB_USER: root
      WORDPRESS_DB_PASSWORD: changeme
      WORDPRESS_DB_NAME: wordpress
    depends_on:
      - db
    restart: unless-stopped
volumes:
  db_data:
`,
	"portainer": `services:
  portainer:
    image: portainer/portainer-ce:latest
    ports:
      - "9000:9000"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - portainer_data:/data
    restart: unless-stopped
volumes:
  portainer_data:
`,
	"openpanel": `services:
  op-db:
    image: postgres:14-alpine
    restart: unless-stopped
    volumes:
      - op-db-data:/var/lib/postgresql/data
    environment:
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: changeme
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U postgres"]
      interval: 10s
      timeout: 5s
      retries: 5

  op-kv:
    image: redis:7.2.5-alpine
    restart: unless-stopped
    volumes:
      - op-kv-data:/data
    command: ["redis-server", "--maxmemory-policy", "noeviction"]
    healthcheck:
      test: ["CMD-SHELL", "redis-cli ping"]
      interval: 10s
      timeout: 5s
      retries: 5

  op-ch:
    image: clickhouse/clickhouse-server:25.10.2.65
    restart: unless-stopped
    volumes:
      - op-ch-data:/var/lib/clickhouse
    environment:
      CLICKHOUSE_DB: openpanel
      CLICKHOUSE_SKIP_USER_SETUP: 1
    ulimits:
      nofile:
        soft: 262144
        hard: 262144
    healthcheck:
      test: ["CMD-SHELL", "clickhouse-client --query 'SELECT 1' || exit 1"]
      interval: 10s
      timeout: 5s
      retries: 5

  op-api:
    image: lindesvard/openpanel-api:2
    restart: unless-stopped
    ports:
      - "3333:3000"
    command: >
      sh -c "
      echo 'Running migrations...';
      CI=true pnpm -r run migrate:deploy;
      pnpm start
      "
    depends_on:
      op-db:
        condition: service_healthy
      op-ch:
        condition: service_healthy
      op-kv:
        condition: service_healthy
    environment:
      NODE_ENV: production
      SELF_HOSTED: "true"
      BATCH_SIZE: "5000"
      BATCH_INTERVAL: "10000"
      ALLOW_REGISTRATION: "false"
      ALLOW_INVITATION: "true"
      REDIS_URL: redis://op-kv:6379
      CLICKHOUSE_URL: http://op-ch:8123/openpanel
      DATABASE_URL: postgresql://postgres:changeme@op-db:5432/postgres?schema=public
      DATABASE_URL_DIRECT: postgresql://postgres:changeme@op-db:5432/postgres?schema=public
      DASHBOARD_URL: http://localhost:3300
      API_URL: http://localhost:3333
      COOKIE_SECRET: changeme_cookie_secret
    healthcheck:
      test: ["CMD-SHELL", "curl -f http://localhost:3000/healthcheck || exit 1"]
      interval: 15s
      timeout: 5s
      retries: 8

  op-dashboard:
    image: lindesvard/openpanel-dashboard:2
    restart: unless-stopped
    ports:
      - "3300:3000"
    depends_on:
      op-api:
        condition: service_healthy
    environment:
      NODE_ENV: production
      SELF_HOSTED: "true"
      REDIS_URL: redis://op-kv:6379
      CLICKHOUSE_URL: http://op-ch:8123/openpanel
      DATABASE_URL: postgresql://postgres:changeme@op-db:5432/postgres?schema=public
      DATABASE_URL_DIRECT: postgresql://postgres:changeme@op-db:5432/postgres?schema=public
      DASHBOARD_URL: http://localhost:3300
      API_URL: http://localhost:3333
      COOKIE_SECRET: changeme_cookie_secret
    healthcheck:
      test: ["CMD-SHELL", "curl -f http://localhost:3000/api/healthcheck || exit 1"]
      interval: 15s
      timeout: 5s
      retries: 8

  op-worker:
    image: lindesvard/openpanel-worker:2
    restart: unless-stopped
    depends_on:
      op-api:
        condition: service_healthy
    environment:
      NODE_ENV: production
      SELF_HOSTED: "true"
      BATCH_SIZE: "5000"
      BATCH_INTERVAL: "10000"
      REDIS_URL: redis://op-kv:6379
      CLICKHOUSE_URL: http://op-ch:8123/openpanel
      DATABASE_URL: postgresql://postgres:changeme@op-db:5432/postgres?schema=public
      DATABASE_URL_DIRECT: postgresql://postgres:changeme@op-db:5432/postgres?schema=public
      DASHBOARD_URL: http://localhost:3300
      API_URL: http://localhost:3333
      COOKIE_SECRET: changeme_cookie_secret
    healthcheck:
      test: ["CMD-SHELL", "curl -f http://localhost:3000/healthcheck || exit 1"]
      interval: 15s
      timeout: 5s
      retries: 8

volumes:
  op-db-data:
  op-kv-data:
  op-ch-data:
`,
}

func ListTemplates() []Template {
	return []Template{
		{ID: "nginx", Name: "Nginx", Description: "Static web server on port 8080"},
		{ID: "mysql", Name: "MySQL 8", Description: "MySQL database with persistent volume"},
		{ID: "redis", Name: "Redis 7", Description: "In-memory cache on port 6379"},
		{ID: "wordpress", Name: "WordPress", Description: "WordPress + MySQL stack on port 8080"},
		{ID: "portainer", Name: "Portainer CE", Description: "Docker management UI on port 9000"},
		{ID: "openpanel", Name: "OpenPanel Analytics", Description: "Product analytics stack — dashboard :3300, API :3333"},
	}
}

func TemplateYAML(id string) (string, error) {
	if id == "" || id == "nginx" {
		return composeTemplates["nginx"], nil
	}
	yaml, ok := composeTemplates[id]
	if !ok {
		return "", fmt.Errorf("未知模板: %s", id)
	}
	return yaml, nil
}
