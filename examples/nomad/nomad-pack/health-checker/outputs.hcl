# -----------------------------------------------------------------------
# Nomad Pack Outputs - Health Checker
# -----------------------------------------------------------------------

output "job_name" {
  description = "Deployed job name"
  value       = var.job_name
}

output "service_name" {
  description = "Consul service name"
  value       = var.service_name
}

output "monitored_service" {
  description = "Systemd service being monitored"
  value       = var.service_to_monitor
}

output "health_endpoint" {
  description = "Health check endpoint"
  value       = "http://localhost:${var.host_port}/health"
}

output "dashboard_url" {
  description = "React dashboard URL"
  value       = "http://localhost:${var.host_port}/"
}

output "metrics_endpoint" {
  description = "Prometheus metrics"
  value       = "http://localhost:${var.host_port}/metrics"
}

output "api_status_endpoint" {
  description = "JSON status API"
  value       = "http://localhost:${var.host_port}/api/status"
}

output "service_discovery" {
  description = "Service discovery details"
  value = {
    consul_service = var.service_name
    consul_dns     = "${var.service_name}.service.consul"
    tags           = var.service_tags
  }
}
