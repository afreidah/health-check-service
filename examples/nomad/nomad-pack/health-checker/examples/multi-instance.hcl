# Multi-Instance Deployment
job_name            = "health-checker-distributed"
datacenters         = ["dc1", "dc2", "dc3"]
task_count          = 3
service_to_monitor  = "nginx"
host_port           = 18080
service_name        = "health-checker"
update_strategy     = "canary"
canary_count        = 1
traefik_enabled     = true
traefik_host        = "health.example.com"
service_tags        = ["monitoring", "distributed", "redundant", "lang:go"]
labels = {
  "app"        = "health-checker"
  "deployment" = "multi-instance"
  "redundancy" = "high"
}
