# Development/Testing Deployment
job_name                   = "health-checker-dev"
service_to_monitor         = "nginx"
host_port                  = 28080
service_name               = "health-checker-dev"
cpu                        = 100
memory                     = 64
log_driver                 = "json-file"
traefik_enabled            = false
update_strategy            = "rolling"
restart_attempts           = 1
nomad_health_interval      = "30s"
service_tags               = ["dev", "test", "temporary"]
labels = {
  "app" = "health-checker"
  "env" = "development"
}
