# Monitor Nginx with Default Settings
job_name            = "health-checker"
datacenters         = ["dc1"]
service_to_monitor  = "nginx"
host_port           = 18080
service_name        = "health-checker-nginx"
traefik_enabled     = true
traefik_host        = "health-nginx.example.com"
service_tags        = ["monitoring", "nginx", "lang:go"]
