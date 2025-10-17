# Monitor PostgreSQL (Production)
job_name             = "health-checker-postgresql"
datacenters          = ["dc1", "dc2"]
node_pool            = "database"
service_to_monitor   = "postgresql"
host_port            = 18081
service_name         = "health-checker-postgres"
cpu                  = 300
memory               = 192
update_strategy      = "canary"
canary_count         = 1
traefik_enabled      = true
traefik_host         = "health-postgres.example.com"
traefik_middleware   = "rate-limit@file,auth@file"
restart_attempts     = 3
service_tags         = ["monitoring", "database", "postgresql", "production", "lang:go"]
labels = {
  "app"      = "health-checker"
  "service"  = "postgresql"
  "env"      = "production"
  "critical" = "true"
}
